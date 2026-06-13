package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/storage"
	"github.com/deepkpat/pulse/pkg/types"
	"github.com/google/uuid"
)

type Daemon struct {
	reader  queue.EventQueueReader
	dedup   *cache.Deduplicator
	storage storage.EventStorage
	dlq     queue.DLQWriter
}

func NewDaemon(r queue.EventQueueReader, d *cache.Deduplicator, s storage.EventStorage, dlq queue.DLQWriter) *Daemon {
	return &Daemon{
		reader:  r,
		dedup:   d,
		storage: s,
		dlq:     dlq,
	}
}

// Start runs the continuous polling loop. It blocks until the context is canceled.
func (d *Daemon) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	slog.Info("worker daemon started polling")

	for {
		select {
		case <-ctx.Done():
			slog.Info("worker daemon shutting down cleanly")
			return
		default:
			// tumbling window: blocks for up to 2s OR until 1024 events are ready
			events, err := d.reader.Dequeue(ctx, 1024)
			if err != nil {
				slog.Error("failed to dequeue events", "error", err)
				continue
			}

			// no events in this window, loop again
			if len(events) == 0 {
				continue
			}

			validEvents := make([]types.Event, 0, len(events))
			for _, ev := range events {
				// data type / schema validation (UUID verification)
				if _, err := uuid.Parse(ev.EventID); err != nil {
					slog.Warn("malformed event_id intercepted; diverting to DLQ", "event_id", ev.EventID, "error", err)
					marshaled, _ := json.Marshal(ev)
					if dlqErr := d.dlq.WriteToDLQ(ctx, fmt.Sprintf("invalid UUID format: %v", err), string(marshaled)); dlqErr != nil {
						slog.Error("failed to write to DLQ — message lost", "error", dlqErr, "event_id", ev.EventID)
					}
					continue
					// safely skip adding to validEvents.
					// Commit() will clear it out of redis.
				}

				validEvents = append(validEvents, ev)
			}

			// write to database
			if len(validEvents) > 0 {
				err := d.storage.BulkInsert(ctx, validEvents)
				if err != nil {
					slog.Error("worker context canceled during database backoff loop", "error", err)
					continue // if the context was canceled, loop and let case <-ctx.Done() catch it
				}

				// dedup is marked AFTER successful storage write to prevent event loss on retry
				// if CheckAndSet fails, we conservatively include the event (already written)
				for _, ev := range validEvents {
					if isDup, err := d.dedup.CheckAndSet(ctx, ev.EventID); err != nil {
						slog.Error("redis dedup mark failed post-write", "error", err, "event_id", ev.EventID)
					} else if isDup {
						// this should not normally happen since we haven't marked before write,
						// but handle it defensively.
						slog.Warn("duplicate event detected post-write (race condition?)", "event_id", ev.EventID)
					}
				}

				slog.Info("processed batch successfully", "valid_events", len(validEvents), "dropped", len(events)-len(validEvents))
			}

			// acknowledge the batch in redis
			if err := d.reader.Commit(ctx); err != nil {
				slog.Error("failed to commit batch in redis — batch will be redelivered", "error", err)
			}
		}
	}
}
