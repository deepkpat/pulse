package storage

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/deepkpat/pulse/pkg/telemetry"
	"github.com/deepkpat/pulse/pkg/types"
	"github.com/google/uuid"
)

type ClickHouseStorage struct {
	conn clickhouse.Conn
}

func NewClickHouseStorage(conn clickhouse.Conn) *ClickHouseStorage {
	return &ClickHouseStorage{conn: conn}
}

func (c *ClickHouseStorage) BulkInsert(ctx context.Context, events []types.Event) error {
	if len(events) == 0 {
		return nil
	}

	backoff := 16 * time.Millisecond
	maxBackoff := 8 * time.Second
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.executeBatch(ctx, events)
		if err == nil {
			return nil
		}

		if isPermanentError(err) {
			return fmt.Errorf("permanent clickhouse error: %w", err)
		}

		if attempt < maxRetries-1 {
			slog.Warn("ClickHouse batch insert failed; backing off and retrying",
				"error", err,
				"attempt", attempt+1,
				"max_attempts", maxRetries,
				"backoff", backoff.String(),
				"batch_size", len(events),
			)
		}

		// add jitter to prevent thundering herd
		jitter := time.Duration(rand.Float64() * float64(backoff))
		waitTime := backoff + jitter

		timer := time.NewTimer(waitTime)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
		telemetry.StorageRetries.Inc()
	}

	return fmt.Errorf("failed to insert batch into clickhouse after %d attempts", maxRetries)
}

// isPermanentError returns true if the error is unrecoverable (e.g., auth, schema mismatch).
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	permanentKeywords := []string{
		"schema mismatch",
		"invalid type",
		"auth failed",
		"authentication failed",
		"no such table",
		"table does not exist",
		"column does not exist",
		"access_denied",
		"permission denied",
		"unknown table",
		"syntax error",
	}

	for _, kw := range permanentKeywords {
		if strings.Contains(errMsg, kw) {
			return true
		}
	}
	return false
}

// executeBatch performs an atomic columnar batch append.
// If any single event validation or insertion fails, the entire batch is rejected.
func (c *ClickHouseStorage) executeBatch(ctx context.Context, events []types.Event) error {
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO pulse_events")
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for i, ev := range events {
		// reject the entire batch on malformed UUIDs; routing to DLQ should happen at the ingestion layer.
		parsedUUID, err := uuid.Parse(ev.EventID)
		if err != nil {
			return fmt.Errorf("invalid uuid in batch at index %d: %s: %w", i, ev.EventID, err)
		}

		err = batch.Append(
			parsedUUID,
			ev.EventName,
			ev.UserID,
			ev.Timestamp,
			ev.RequestID,
			ev.Properties,
		)
		if err != nil {
			return fmt.Errorf("failed to append event %d to batch: %w", i, err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to flush batch to clickhouse: %w", err)
	}

	return nil
}
