package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Deduplicator struct {
	client *redis.Client
	ttl    time.Duration
}

// NewDeduplicator initializes our idempotency cache with a 16-minute TTL
func NewDeduplicator(client *redis.Client) *Deduplicator {
	return &Deduplicator{
		client: client,
		ttl:    16 * time.Minute,
	}
}

// CheckAndSet returns true if the event is a duplicate, false if it's new.
func (d *Deduplicator) CheckAndSet(ctx context.Context, eventID string) (bool, error) {
	key := "dedup:event:" + eventID

	// SetNX sets the key only if it does not already exist.
	// isNew will be true if the key was successfully set.
	isNew, err := d.client.SetNX(ctx, key, "1", d.ttl).Result()
	if err != nil {
		return false, err
	}

	// if it's NOT new, it IS a duplicate.
	return !isNew, nil
}
