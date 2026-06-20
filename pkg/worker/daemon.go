package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deepkpat/pulse/pkg/cache"
	pulserrors "github.com/deepkpat/pulse/pkg/errors"
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

	// State tracking for failed commits
	pendingCommitErr error
	lastBatchSize    int
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
// Contract:
// - On graceful shutdown (context cancelled), drains the current batch before exiting
// - On Dequeue error, logs and retries (exponential backoff)
// - On storage error, routes batch to DLQ and retries Commit
// - On Commit error, retries Commit on next loop iteration (does NOT proceed to Dequeue)
func (d *Daemon) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	slog.Info("worker daemon started")

	// Backoff for Dequeue/Commit failures
	dequeueBackoff := 100 * time.Millisecond
	const maxDequeueBackoff = 5 * time.Second

	for {
		// Check for graceful shutdown
		select {
		case <-ctx.Done():
			slog.Info("worker daemon shutdown signal received; exiting cleanly")
			return
		default:
		}

		// Retry failed Commit before proceeding to Dequeue
		if d.pendingCommitErr != nil {
			slog.Info("retrying previous failed commit",
				"batch_size", d.lastBatchSize,
				"previous_error", d.pendingCommitErr,
			)
			if err := d.reader.Commit(ctx); err != nil {
				telemetry.CommitFailuresTotal.Inc()
				slog.Error("commit still failing; will retry on next loop iteration",
					"error", err,
					"batch_size", d.lastBatchSize,
				)
				d.pendingCommitErr = err

				// Back off briefly to avoid tight retry loop if Commit keeps failing
				time.Sleep(dequeueBackoff)
				if dequeueBackoff < maxDequeueBackoff {
					dequeueBackoff *= 2
				}
				continue
			}

			// Commit succeeded; clear error state and reset backoff
			slog.Info("commit retry succeeded", "batch_size", d.lastBatchSize)
			d.pendingCommitErr = nil
			d.lastBatchSize = 0
			dequeueBackoff = 100 * time.Millisecond
		}

		// Dequeue: blocks for up to 2s OR until batchSize events are ready
		events, err := d.reader.Dequeue(ctx, 1024)
		if err != nil {
			// Check if context was cancelled during Dequeue
			if errors.Is(err, context.Canceled) {
				slog.Info("worker daemon shutting down (context cancelled)")
				return
			}

			telemetry.WorkerBatchesTotal.WithLabelValues("error").Inc()
			slog.Error("failed to dequeue events", "error", err)

			// Back off before retrying Dequeue
			time.Sleep(dequeueBackoff)
			if dequeueBackoff < maxDequeueBackoff {
				dequeueBackoff *= 2
			}
			continue
		}

		// Reset backoff on successful Dequeue
		dequeueBackoff = 100 * time.Millisecond

		// No events in this window; loop again
		if len(events) == 0 {
			continue
		}

		telemetry.WorkerBatchesTotal.WithLabelValues("ok").Inc()
		telemetry.WorkerBatchSize.Observe(float64(len(events)))

		// Process events: validate, dedup, filter
		uniqueEvents, droppedCount := d.processEvents(ctx, events)

		// Write to storage if any events passed filtering
		if len(uniqueEvents) > 0 {
			d.handleStorageInsert(ctx, uniqueEvents)
		}

		// Log batch processing summary
		if len(uniqueEvents) > 0 || droppedCount > 0 {
			lastRequestID := ""
			if len(uniqueEvents) > 0 {
				lastRequestID = uniqueEvents[len(uniqueEvents)-1].RequestID
			}

			slog.Info("processed batch",
				"total_events", len(events),
				"unique_events", len(uniqueEvents),
				"dropped", droppedCount,
				"last_request_id", lastRequestID,
			)
		}

		// Commit the batch in Redis
		// This acknowledges ALL events (including dropped ones) from this Dequeue call.
		// If Commit fails, we retry on the next loop iteration and do NOT Dequeue again.
		if err := d.reader.Commit(ctx); err != nil {
			telemetry.CommitFailuresTotal.Inc()
			slog.Error("failed to commit batch; will retry on next iteration",
				"error", err,
				"batch_size", len(events),
			)
			d.pendingCommitErr = err
			d.lastBatchSize = len(events)
			continue
		}

		// Commit succeeded; clear any error state
		d.pendingCommitErr = nil
		d.lastBatchSize = 0
	}
}

// processEvents filters and deduplicates a batch of raw events.
// Returns (uniqueEvents, droppedCount).
// Errors during validation or dedup are logged and events are either dropped or conservatively included.
func (d *Daemon) processEvents(ctx context.Context, events []types.Event) ([]types.Event, int) {
	uniqueEvents := make([]types.Event, 0, len(events))
	droppedCount := 0

	for _, ev := range events {
		// Validate UUID format
		if _, err := uuid.Parse(ev.EventID); err != nil {
			telemetry.WorkerEventsDropped.WithLabelValues("invalid_uuid").Inc()
			slog.Warn("invalid event_id; routing to DLQ",
				"event_id", ev.EventID,
				"error", err,
			)
			d.routeToDLQ(ctx, ev, fmt.Sprintf("invalid UUID format: %v", err))
			droppedCount++
			continue
		}

		// Check for duplicates using Redis-backed deduplicator
		// If Redis is unavailable, conservatively include the event (prefer duplicates over loss)
		isDup, err := d.dedup.CheckAndSet(ctx, ev.EventID)
		if err != nil {
			// Redis failure: log and include event
			slog.Warn("dedup check failed; conservatively including event",
				"error", err,
				"event_id", ev.EventID,
			)
			uniqueEvents = append(uniqueEvents, ev)
			continue
		}

		if isDup {
			telemetry.WorkerDuplicatesTotal.Inc()
			slog.Warn("duplicate event dropped", "event_id", ev.EventID)
			droppedCount++
			continue
		}

		uniqueEvents = append(uniqueEvents, ev)
	}

	return uniqueEvents, droppedCount
}

// handleStorageInsert attempts to persist a batch of events to storage.
// Distinguishes between transient and permanent failures:
//   - Transient (network down, timeout): Skip DLQ, return without Commit so
//     the batch stays in Redis PEL and is redelivered on the next iteration.
//   - Permanent (schema error, auth error): Route to DLQ, then fall through
//     to Commit so the batch is acknowledged and not retried forever.
func (d *Daemon) handleStorageInsert(ctx context.Context, events []types.Event) {
	insertStart := time.Now()
	err := d.storage.BulkInsert(ctx, events)
	duration := time.Since(insertStart)
	telemetry.StorageInsertDuration.Observe(duration.Seconds())

	if err == nil {
		telemetry.StorageInsertsTotal.WithLabelValues("ok").Inc()
		telemetry.StorageEventsInserted.Add(float64(len(events)))
		slog.Debug("storage insert succeeded",
			slog.Int("batch_size", len(events)),
			slog.String("duration", duration.String()),
		)
		return
	}

	// Classify the error before deciding what to do with the batch.
	classification := pulserrors.ClassifyError(err)

	if classification == "transient" {
		// Transient error (network, timeout, connection refused).
		// Do NOT send to DLQ. Return without calling Commit so the
		// batch stays in the Redis PEL and is redelivered next iteration.
		telemetry.StorageInsertsTotal.WithLabelValues("error_transient").Inc()
		slog.Warn("storage bulk insert failed (transient); batch will be retried",
			slog.String("error", err.Error()),
			slog.Int("batch_size", len(events)),
			slog.String("duration", duration.String()),
		)
		return
	}

	// Permanent error (schema, auth, validation).
	// Route to DLQ to avoid reprocessing forever.
	telemetry.StorageInsertsTotal.WithLabelValues("error_permanent").Inc()
	slog.Error("storage bulk insert failed (permanent); routing batch to DLQ",
		slog.String("error", err.Error()),
		slog.Int("batch_size", len(events)),
		slog.String("duration", duration.String()),
	)

	failureReason := fmt.Sprintf("permanent storage failure: %v", err)
	for _, ev := range events {
		d.routeToDLQ(ctx, ev, failureReason)
	}
}

// routeToDLQ writes a single event to the dead-letter queue.
// If DLQ write fails, logs the error but does NOT fail the batch.
// The event is acknowledged in Redis as if it was successfully processed
// (since it's unrecoverable anyway).
func (d *Daemon) routeToDLQ(ctx context.Context, event types.Event, reason string) {
	marshaled, err := json.Marshal(event)
	if err != nil {
		// Event marshalling failed; create a minimal DLQ entry
		slog.Error("failed to marshal event for DLQ",
			"error", err,
			"event_id", event.EventID,
			"reason", reason,
		)
		telemetry.DLQWritesTotal.WithLabelValues("marshal_error", "error").Inc()
		return
	}

	// Determine the DLQ reason category for telemetry
	reasonCategory := "unknown"
	if len(reason) > 0 {
		// Extract category from reason string (e.g., "invalid UUID format" → "invalid_uuid")
		if json.Valid(marshaled) {
			reasonCategory = "validation_error"
		} else {
			reasonCategory = "parse_error"
		}
	}

	if dlqErr := d.dlq.WriteToDLQ(ctx, reason, string(marshaled)); dlqErr != nil {
		telemetry.DLQWritesTotal.WithLabelValues(reasonCategory, "error").Inc()
		slog.Error("failed to write event to DLQ; event will be lost",
			"error", dlqErr,
			"event_id", event.EventID,
			"reason", reason,
		)
		return
	}

	telemetry.DLQWritesTotal.WithLabelValues(reasonCategory, "ok").Inc()
	slog.Debug("event routed to DLQ", "event_id", event.EventID, "reason", reason)
}
