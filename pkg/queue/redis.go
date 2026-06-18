package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deepkpat/pulse/pkg/types"
	"github.com/redis/go-redis/v9"
)

// RedisQueue implements EventQueue, EventQueueReader, and DLQWriter using Redis Streams.
type RedisQueue struct {
	client       *redis.Client
	streamName   string
	groupName    string
	consumerName string

	mu          sync.Mutex
	lastReadIDs []string
}

func NewRedisQueue(client *redis.Client, streamName, groupName, consumerName string) *RedisQueue {
	return &RedisQueue{
		client:       client,
		streamName:   streamName,
		groupName:    groupName,
		consumerName: consumerName,
	}
}

// Enqueue appends a single event to the end of the queue.
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

// Dequeue retrieves up to batchSize events. It prioritizes pending (PEL) messages
// before blocking on new incoming messages.
func (r *RedisQueue) Dequeue(ctx context.Context, batchSize uint64) ([]types.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. check for pending messages that were never acknowledged (ID "0")
	events, ids, err := r.fetchFromStreamLocked(ctx, "0", batchSize, 0)
	if err != nil {
		return nil, err
	}

	if len(events) > 0 {
		r.lastReadIDs = ids
		return events, nil
	}

	// 2. fetch new messages, blocking for up to 2 seconds if empty (ID ">")
	events, ids, err = r.fetchFromStreamLocked(ctx, ">", batchSize, 2*time.Second)
	if err != nil {
		return nil, err
	}
	r.lastReadIDs = ids
	return events, nil
}

// fetchFromStreamLocked encapsulates XReadGroup logic. Must be called under mutex lock.
func (r *RedisQueue) fetchFromStreamLocked(ctx context.Context, id string, batchSize uint64, blockTime time.Duration) ([]types.Event, []string, error) {
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.groupName,
		Consumer: r.consumerName,
		Streams:  []string{r.streamName, id},
		Count:    int64(batchSize),
		Block:    blockTime,
	}).Result()

	if err == redis.Nil {
		return nil, nil, nil
	}
	if errors.Is(err, context.Canceled) {
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, fmt.Errorf("xreadgroup failed: %w", err)
	}

	var events []types.Event
	var messageIDs []string

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			rawData, ok := msg.Values["data"].(string)
			if !ok {
				// malformed message
				payload := fmt.Sprintf("%v", msg.Values)
				if dlqErr := r.WriteToDLQ(ctx, "malformed stream item: missing or non-string data field", payload); dlqErr != nil {
					slog.Error("failed to write malformed message to DLQ; message will be retried",
						"error", dlqErr,
						"message_id", msg.ID,
					)
					continue
				}
				messageIDs = append(messageIDs, msg.ID)
				continue
			}

			var event types.Event
			if err := json.Unmarshal([]byte(rawData), &event); err != nil {
				// JSON parse failure
				dlqErr := r.WriteToDLQ(ctx, fmt.Sprintf("json unmarshal error: %v", err), rawData)
				if dlqErr != nil {
					slog.Error("failed to write unparseable message to DLQ; message will be retried",
						"error", dlqErr,
						"message_id", msg.ID,
					)
					continue
				}
				messageIDs = append(messageIDs, msg.ID)
				continue
			}

			events = append(events, event)
			messageIDs = append(messageIDs, msg.ID)
		}
	}

	return events, messageIDs, nil
}

// Commit acknowledges the successful processing of the previously fetched batch.
func (r *RedisQueue) Commit(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.lastReadIDs) == 0 {
		return nil
	}

	if err := r.client.XAck(ctx, r.streamName, r.groupName, r.lastReadIDs...).Err(); err != nil {
		slog.Error("XAck failed; batch will be redelivered on next dequeue",
			"error", err,
			"batch_size", len(r.lastReadIDs),
		)
		return fmt.Errorf("failed to acknowledge batch: %w", err)
	}

	// clear ONLY after successful XAck
	r.lastReadIDs = []string{}
	return nil
}

// WriteToDLQ pushes unprocessable items into a dedicated stream suffixed with "_dlq".
func (r *RedisQueue) WriteToDLQ(ctx context.Context, reason string, payload string) error {
	dlqStream := r.streamName + "_dlq"
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: map[string]interface{}{
			"reason":    reason,
			"payload":   payload,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}).Err()
}

// compile-time assertions verifying interface implementation.
var _ EventQueue = (*RedisQueue)(nil)
var _ EventQueueReader = (*RedisQueue)(nil)
var _ DLQWriter = (*RedisQueue)(nil)
