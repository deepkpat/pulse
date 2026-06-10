package storage

import (
	"context"

	"github.com/deepkpat/pulse/pkg/types"
)

// EventStorage defines the behavior for persisting batches of events.
type EventStorage interface {
	// BulkInsert writes a batch of enriched events to the underlying datastore.
	BulkInsert(ctx context.Context, events []types.Event) error
}
