package queue

import (
	"context"

	"github.com/deepkpat/pulse/pkg/types"
)

// EventQueue defines the behavior for writing events to a queue.
type EventQueue interface {
	// Enqueue appends a single event to the end of the queue.
	Enqueue(ctx context.Context, event types.Event) error
}

// EventQueueReader defines the behavior for consuming and acknowledging events.
type EventQueueReader interface {
	// Dequeue retrieves up to batchSize events from the front of the queue.
	Dequeue(ctx context.Context, batchSize uint64) ([]types.Event, error)
	// Commit acknowledges the successful processing of the previously fetched batch.
	Commit(ctx context.Context)
}
