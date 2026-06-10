package worker

import (
	"context"
	"log/slog"
	"sync"

	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/storage"
	"github.com/deepkpat/pulse/pkg/types"
)

type Daemon struct {
	reader  queue.EventQueueReader
	dedup   *cache.Deduplicator
	storage storage.EventStorage
}

func NewDaemon(r queue.EventQueueReader, d *cache.Deduplicator, s storage.EventStorage) *Daemon {
	return &Daemon{
		reader:  r,
		dedup:   d,
		storage: s,
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

			// idempotency check
			validEvents := make([]types.Event, 0, len(events))
			for _, ev := range events {
				isDup, err := d.dedup.CheckAndSet(ctx, ev.EventID)
				if err != nil {
					slog.Error("redis dedup check failed", "error", err, "event_id", ev.EventID)
					// fail-safe: if redis fails, we append it anyway.
					// better to have a rare duplicate than lose user data.
					validEvents = append(validEvents, ev)
					continue
				}

				if isDup {
					slog.Debug("dropped duplicate event", "event_id", ev.EventID)
					continue
				}

				validEvents = append(validEvents, ev)
			}

			// TODO: write to database (placeholder)
			if len(validEvents) > 0 {
				err := d.storage.BulkInsert(ctx, validEvents)
				if err != nil {
					slog.Error("worker context canceled during database backoff loop", "error", err)
					continue // if the context was canceled, loop and let case <-ctx.Done() catch it
				}
				slog.Info("processed batch successfully", "valid_events", len(validEvents), "dropped", len(events)-len(validEvents))
			}

			// acknowledge the batch in Redis
			d.reader.Commit(ctx)
		}
	}
}
