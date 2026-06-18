package storage

import (
	"context"
	"fmt"
	"log/slog"
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

// NewClickHouseStorage establishes a connection pool to ClickHouse.
func NewClickHouseStorage(addr, user, password, database string) (*ClickHouseStorage, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: database,
			Username: user,
			Password: password,
		},
		DialTimeout: 4 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse connection failed: %w", err)
	}

	return &ClickHouseStorage{conn: conn}, nil
}

// BulkInsert pushes a batch of events to ClickHouse using native high-performance batching.
// It features a bounded exponential backoff loop to fulfill RFC's backpressure constraint.
func (c *ClickHouseStorage) BulkInsert(ctx context.Context, events []types.Event) error {
	if len(events) == 0 {
		return nil
	}

	backoff := 128 * time.Millisecond
	maxBackoff := 32 * time.Second
	maxRetries := 4

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := c.executeBatch(ctx, events)
			if err == nil {
				return nil // success! exit retry loop
			}

			// if it's a permanent error (like schema mismatch or auth), don't retry
			if isPermanent(err) {
				return fmt.Errorf("permanent clickhouse error: %w", err)
			}

			slog.Error("ClickHouse batch insert failed. Backing off before retry.",
				"error", err,
				"retry_count", i+1,
				"retry_delay", backoff.String(),
				"batch_size", len(events),
			)

			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}

			backoff *= 2
			telemetry.StorageRetries.Inc()
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return fmt.Errorf("failed to insert batch into clickhouse after %d retries", maxRetries)
}

// isPermanent returns true if the error is considered non-retryable.
func isPermanent(err error) bool {
	// this is a heuristic. in production, we would check for specific ClickHouse error codes (like 170, 43 etc)
	errMsg := err.Error()
	// common non-retryable strings
	permanentKeywords := []string{
		"schema mismatch",
		"invalid type",
		"auth failed",
		"authentication failed",
		"no such table",
		"table does not exist",
		"column does not exist",
		"ACCESS_DENIED",
	}

	for _, kw := range permanentKeywords {
		if strings.Contains(strings.ToLower(errMsg), strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// executeBatch performs the raw columnar batch append operation
func (c *ClickHouseStorage) executeBatch(ctx context.Context, events []types.Event) error {
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO pulse_events")
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, ev := range events {
		// ClickHouse expects native string representation of UUIDs parsed into an actual UUID object
		parsedUUID, err := uuid.Parse(ev.EventID)
		if err != nil {
			// return error instead of silently skipping, to avoid silent data loss
			return fmt.Errorf("invalid uuid in batch: %s: %w", ev.EventID, err)
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
			return fmt.Errorf("failed to append to batch: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to flush batch to clickhouse: %w", err)
	}

	return nil
}
