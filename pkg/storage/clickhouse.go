package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
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
// It features an infinite exponential backoff loop to fulfill RFC's backpressure constraint.
func (c *ClickHouseStorage) BulkInsert(ctx context.Context, events []types.Event) error {
	if len(events) == 0 {
		return nil
	}

	backoff := 128 * time.Millisecond
	maxBackoff := 32 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := c.executeBatch(ctx, events)
			if err == nil {
				return nil // success! exit retry loop
			}

			slog.Error("ClickHouse batch insert failed. Backing off before retry.",
				"error", err,
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
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
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
			slog.Warn("malformed event_id skipped during batch compilation", "event_id", ev.EventID, "error", err)
			continue
		}

		err = batch.Append(
			parsedUUID,
			ev.EventName,
			ev.UserID,
			ev.Timestamp,
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
