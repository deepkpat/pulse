package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"time"

	"github.com/deepkpat/pulse/pkg/cache"
	"github.com/deepkpat/pulse/pkg/queue"
	"github.com/deepkpat/pulse/pkg/storage"
	"github.com/deepkpat/pulse/pkg/telemetry"
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
				telemetry.WorkerBatchesTotal.WithLabelValues("error").Inc()
				slog.Error("failed to dequeue events", "error", err)
				continue
			}

			// no events in this window, loop again
			if len(events) == 0 {
				continue
			}

			telemetry.WorkerBatchesTotal.WithLabelValues("ok").Inc()
			telemetry.WorkerBatchSize.Observe(float64(len(events)))

			uniqueEvents := make([]types.Event, 0, len(events))
			for _, ev := range events {
				// data type / schema validation (UUID verification)
				if _, err := uuid.Parse(ev.EventID); err != nil {
					telemetry.WorkerEventsDropped.WithLabelValues("invalid_uuid").Inc()
					slog.Warn("malformed event_id intercepted; diverting to DLQ", "event_id", ev.EventID, "error", err)
					marshaled, _ := json.Marshal(ev)
					if dlqErr := d.dlq.WriteToDLQ(ctx, fmt.Sprintf("invalid UUID format: %v", err), string(marshaled)); dlqErr != nil {
						telemetry.DLQWritesTotal.WithLabelValues("invalid_uuid", "error").Inc()
						slog.Error("failed to write to DLQ — message lost", "error", dlqErr, "event_id", ev.EventID)
					} else {
						telemetry.DLQWritesTotal.WithLabelValues("invalid_uuid", "ok").Inc()
					}
					continue
				}

				// check-and-set BEFORE storage write to prevent duplicate rows in ClickHouse
				// if redis fails, we conservatively include the event (prefer duplicates over loss)
				isDup, err := d.dedup.CheckAndSet(ctx, ev.EventID)
				if err != nil {
					slog.Error("redis dedup check failed; conservatively including event", "error", err, "event_id", ev.EventID)
					uniqueEvents = append(uniqueEvents, ev)
					continue
				}

				if isDup {
					telemetry.WorkerDuplicatesTotal.Inc()
					slog.Warn("duplicate event detected and dropped", "event_id", ev.EventID)
					continue
				}

				uniqueEvents = append(uniqueEvents, ev)
			}

			// write to database
			if len(uniqueEvents) > 0 {
				insertStart := time.Now()
				err := d.storage.BulkInsert(ctx, uniqueEvents)
				telemetry.StorageInsertDuration.Observe(time.Since(insertStart).Seconds())

				if err != nil {
					telemetry.StorageInsertsTotal.WithLabelValues("error").Inc()
					slog.Error("bulk insert failed permanently; diverting to DLQ", "error", err, "batch_size", len(uniqueEvents))

					// move the entire failed batch to DLQ if storage fails permanently
					for _, ev := range uniqueEvents {
						marshaled, _ := json.Marshal(ev)
						if dlqErr := d.dlq.WriteToDLQ(ctx, fmt.Sprintf("storage failure: %v", err), string(marshaled)); dlqErr != nil {
							telemetry.DLQWritesTotal.WithLabelValues("storage_error", "error").Inc()
						} else {
							telemetry.DLQWritesTotal.WithLabelValues("storage_error", "ok").Inc()
						}
					}
					// we continue to Commit() so this "poison batch" isn't retried forever
				} else {
					telemetry.StorageInsertsTotal.WithLabelValues("ok").Inc()
					telemetry.StorageEventsInserted.Add(float64(len(uniqueEvents)))
				}

				slog.Info("processed batch successfully",
					"unique_events", len(uniqueEvents),
					"dropped", len(events)-len(uniqueEvents),
					"last_request_id", uniqueEvents[len(uniqueEvents)-1].RequestID,
				)
			}

			// acknowledge the batch in redis
			if err := d.reader.Commit(ctx); err != nil {
				telemetry.CommitFailuresTotal.Inc()
				slog.Error("failed to commit batch in redis — batch will be redelivered", "error", err)
			}
		}
	}
}
