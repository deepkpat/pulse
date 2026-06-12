package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deepkpat/pulse/pkg/types"
	"github.com/redis/go-redis/v9"
)

type targetID string

// RedisQueue implements EventQueue and EventQueueReader using Redis Streams and consumer groups.
type RedisQueue struct {
	client       *redis.Client
	streamName   string
	groupName    string
	consumerName string
	lastReadIDs  []string // tracks unacknowledged message IDs for Commit()
}

// NewRedisQueue initializes a new RedisQueue instance.
func NewRedisQueue(client *redis.Client, streamName, groupName, consumerName string) *RedisQueue {
	return &RedisQueue{
		client:       client,
		streamName:   streamName,
		groupName:    groupName,
		consumerName: consumerName,
	}
}

// Enqueue marshals the event and appends it to the Redis stream using XADD.
func (r *RedisQueue) Enqueue(ctx context.Context, event types.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamName,
		Values: map[string]interface{}{"data": data},
	}).Err()
}

// Dequeue fetches messages from the stream. It prioritizes recovering uncommitted
// messages ("0") from previous runs before fetching brand-new messages (">").
func (r *RedisQueue) Dequeue(ctx context.Context, batchSize uint64) ([]types.Event, error) {
	r.lastReadIDs = []string{}

	// check for pending, uncommitted messages assigned to this consumer
	events, err := r.fetchFromStream(ctx, "0", batchSize, 0)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		return events, nil
	}

	// fall back to pulling new messages if no pending work exists
	return r.fetchFromStream(ctx, ">", batchSize, 2*time.Second)
}

// fetchFromStream is a helper executing XREADGROUP and handling payload decoding/DLQ routing.
func (r *RedisQueue) fetchFromStream(ctx context.Context, id targetID, batchSize uint64, blockTime time.Duration) ([]types.Event, error) {
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.groupName,
		Consumer: r.consumerName,
		Streams:  []string{r.streamName, string(id)},
		Count:    int64(batchSize),
		Block:    blockTime,
	}).Result()

	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var events []types.Event
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			rawData, ok := msg.Values["data"].(string)
			if !ok {
				_ = r.WriteToDLQ(ctx, "malformed stream item: missing data field", fmt.Sprintf("%v", msg.Values))
				r.lastReadIDs = append(r.lastReadIDs, msg.ID)
				continue
			}

			var event types.Event
			if err := json.Unmarshal([]byte(rawData), &event); err != nil {
				_ = r.WriteToDLQ(ctx, fmt.Sprintf("json unmarshal error: %v", err), rawData)
				r.lastReadIDs = append(r.lastReadIDs, msg.ID)
				continue
			}

			events = append(events, event)
			r.lastReadIDs = append(r.lastReadIDs, msg.ID)
		}
	}

	return events, nil
}

// Commit acknowledges the current batch of messages using XACK.
func (r *RedisQueue) Commit(ctx context.Context) {
	if len(r.lastReadIDs) > 0 {
		r.client.XAck(ctx, r.streamName, r.groupName, r.lastReadIDs...)
		r.lastReadIDs = []string{}
	}
}

// WriteToDLQ writes toxic, unparseable payloads to a secondary stream suffixed with "_dlq".
func (r *RedisQueue) WriteToDLQ(ctx context.Context, reason string, payload string) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamName + "_dlq",
		Values: map[string]interface{}{
			"reason":    reason,
			"payload":   payload,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}).Err()
}
