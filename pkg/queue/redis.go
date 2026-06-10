package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deepkpat/pulse/pkg/types"
	"github.com/redis/go-redis/v9"
)

// RedisQueue implements both EventQueue and EventQueueReader interfaces
// using Redis Streams. It utilizes consumer groups to allow multiple
// workers to distribute the processing load concurrently.
type RedisQueue struct {
	client       *redis.Client
	streamName   string
	groupName    string
	consumerName string
	// internal state to track IDs for the Commit() method
	lastReadIDs []string
}

// NewRedisQueue initializes the queue.
// groupName should be constant across workers.
// consumerName should be unique (e.g., hostname + random ID).
func NewRedisQueue(client *redis.Client, streamName, groupName, consumerName string) *RedisQueue {
	return &RedisQueue{
		client:       client,
		streamName:   streamName,
		groupName:    groupName,
		consumerName: consumerName,
	}
}

// Enqueue appends an event to the stream using XADD.
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

// Dequeue retrieves events using XREADGROUP.
func (r *RedisQueue) Dequeue(ctx context.Context, batchSize uint64) ([]types.Event, error) {
	// ">" means read new messages not yet delivered to others
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.groupName,
		Consumer: r.consumerName,
		Streams:  []string{r.streamName, ">"},
		Count:    int64(batchSize),
		Block:    2 * time.Second, // block briefly to wait for events
	}).Result()

	if err == redis.Nil {
		return nil, nil // no new messages
	}
	if err != nil {
		return nil, err
	}

	var events []types.Event
	r.lastReadIDs = []string{}

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			rawData, ok := msg.Values["data"].(string)
			if !ok {
				// the payload doesn't even contain a string data field
				_ = r.WriteToDLQ(ctx, "malformed stream item: missing data field", fmt.Sprintf("%v", msg.Values))
				r.lastReadIDs = append(r.lastReadIDs, msg.ID)
				continue
			}

			var event types.Event
			if err := json.Unmarshal([]byte(rawData), &event); err != nil {
				// poison pill protection:
				// route directly to DLQ and mark for ACK so it doesn't stall the stream
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

// Commit acknowledges the successful processing of the batch.
func (r *RedisQueue) Commit(ctx context.Context) {
	if len(r.lastReadIDs) > 0 {
		r.client.XAck(ctx, r.streamName, r.groupName, r.lastReadIDs...)
		r.lastReadIDs = []string{}
	}
}

// WriteToDLQ pushes toxic payloads to a secondary stream
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
