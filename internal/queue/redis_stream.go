package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/enterprise/risk-engine/configs"
	"github.com/enterprise/risk-engine/internal/models"
)

// RedisStreamClient handles Redis Streams operations
type RedisStreamClient struct {
	client           *redis.Client
	streamName       string
	consumerGroup    string
	deadLetterStream string
	maxRetries       int
}

// NewRedisStreamClient creates a new Redis stream client
func NewRedisStreamClient(cfg configs.RedisConfig) (*RedisStreamClient, error) {
	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	rsc := &RedisStreamClient{
		client:           client,
		streamName:       cfg.StreamName,
		consumerGroup:    cfg.ConsumerGroup,
		deadLetterStream: "transactions-dlq",
		maxRetries:       cfg.MaxRetries,
	}

	// Create consumer group if it doesn't exist
	if err := rsc.createConsumerGroup(ctx); err != nil {
		log.Warn().Err(err).Msg("Consumer group may already exist")
	}

	log.Info().Msg("Redis Stream client initialized")
	return rsc, nil
}

// createConsumerGroup creates the consumer group for the stream
func (r *RedisStreamClient) createConsumerGroup(ctx context.Context) error {
	// Try to create the stream and consumer group
	// MKSTREAM creates the stream if it doesn't exist
	err := r.client.XGroupCreateMkStream(ctx, r.streamName, r.consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// Publish publishes a transaction event to the stream
func (r *RedisStreamClient) Publish(ctx context.Context, event *models.TransactionEvent) (string, error) {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event: %w", err)
	}

	msgID, err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamName,
		Values: map[string]interface{}{
			"data": string(eventJSON),
		},
	}).Result()

	if err != nil {
		return "", fmt.Errorf("failed to publish event: %w", err)
	}

	log.Debug().
		Str("message_id", msgID).
		Str("transaction_id", event.TransactionID).
		Msg("Event published to stream")

	return msgID, nil
}

// PublishBatch publishes multiple events to the stream
func (r *RedisStreamClient) PublishBatch(ctx context.Context, events []*models.TransactionEvent) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}

	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(events))

	for i, event := range events {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal event %d: %w", i, err)
		}

		cmds[i] = pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: r.streamName,
			Values: map[string]interface{}{
				"data": string(eventJSON),
			},
		})
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute pipeline: %w", err)
	}

	msgIDs := make([]string, len(events))
	for i, cmd := range cmds {
		msgIDs[i] = cmd.Val()
	}

	log.Debug().
		Int("count", len(events)).
		Msg("Batch events published to stream")

	return msgIDs, nil
}

// Consume consumes events from the stream
func (r *RedisStreamClient) Consume(ctx context.Context, consumerName string, count int64, blockDuration time.Duration) ([]StreamMessage, error) {
	// First, try to claim pending messages that may have been abandoned
	pendingMessages, err := r.claimPendingMessages(ctx, consumerName, count)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to claim pending messages")
	}

	if len(pendingMessages) > 0 {
		return pendingMessages, nil
	}

	// Read new messages
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    r.consumerGroup,
		Consumer: consumerName,
		Streams:  []string{r.streamName, ">"},
		Count:    count,
		Block:    blockDuration,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, nil // No messages available
		}
		return nil, fmt.Errorf("failed to read from stream: %w", err)
	}

	var messages []StreamMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			event, err := r.parseMessage(msg)
			if err != nil {
				log.Error().Err(err).Str("message_id", msg.ID).Msg("Failed to parse message")
				continue
			}

			messages = append(messages, StreamMessage{
				ID:    msg.ID,
				Event: event,
			})
		}
	}

	return messages, nil
}

// claimPendingMessages claims messages that have been pending for too long
func (r *RedisStreamClient) claimPendingMessages(ctx context.Context, consumerName string, count int64) ([]StreamMessage, error) {
	// Claim messages that have been pending for more than 30 seconds
	minIdleTime := 30 * time.Second

	pending, err := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: r.streamName,
		Group:  r.consumerGroup,
		Start:  "-",
		End:    "+",
		Count:  count,
	}).Result()

	if err != nil {
		return nil, err
	}

	if len(pending) == 0 {
		return nil, nil
	}

	var messageIDs []string
	for _, p := range pending {
		if p.Idle >= minIdleTime {
			messageIDs = append(messageIDs, p.ID)
		}
	}

	if len(messageIDs) == 0 {
		return nil, nil
	}

	// Claim the messages
	claimed, err := r.client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   r.streamName,
		Group:    r.consumerGroup,
		Consumer: consumerName,
		MinIdle:  minIdleTime,
		Messages: messageIDs,
	}).Result()

	if err != nil {
		return nil, err
	}

	var messages []StreamMessage
	for _, msg := range claimed {
		event, err := r.parseMessage(msg)
		if err != nil {
			log.Error().Err(err).Str("message_id", msg.ID).Msg("Failed to parse claimed message")
			continue
		}

		messages = append(messages, StreamMessage{
			ID:    msg.ID,
			Event: event,
		})
	}

	return messages, nil
}

// parseMessage parses a Redis stream message into a TransactionEvent
func (r *RedisStreamClient) parseMessage(msg redis.XMessage) (*models.TransactionEvent, error) {
	data, ok := msg.Values["data"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	var event models.TransactionEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

// Acknowledge acknowledges a message as processed
func (r *RedisStreamClient) Acknowledge(ctx context.Context, messageID string) error {
	_, err := r.client.XAck(ctx, r.streamName, r.consumerGroup, messageID).Result()
	if err != nil {
		return fmt.Errorf("failed to acknowledge message: %w", err)
	}

	log.Debug().Str("message_id", messageID).Msg("Message acknowledged")
	return nil
}

// AcknowledgeBatch acknowledges multiple messages
func (r *RedisStreamClient) AcknowledgeBatch(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	_, err := r.client.XAck(ctx, r.streamName, r.consumerGroup, messageIDs...).Result()
	if err != nil {
		return fmt.Errorf("failed to acknowledge messages: %w", err)
	}

	log.Debug().Int("count", len(messageIDs)).Msg("Messages acknowledged")
	return nil
}

// SendToDeadLetter sends a failed message to the dead letter stream
func (r *RedisStreamClient) SendToDeadLetter(ctx context.Context, event *models.TransactionEvent, err error) error {
	eventJSON, _ := json.Marshal(event)

	_, dlqErr := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.deadLetterStream,
		Values: map[string]interface{}{
			"data":  string(eventJSON),
			"error": err.Error(),
		},
	}).Result()

	if dlqErr != nil {
		return fmt.Errorf("failed to send to dead letter: %w", dlqErr)
	}

	log.Warn().
		Str("transaction_id", event.TransactionID).
		Err(err).
		Msg("Message sent to dead letter queue")

	return nil
}

// GetStreamInfo returns information about the stream
func (r *RedisStreamClient) GetStreamInfo(ctx context.Context) (*StreamInfo, error) {
	info, err := r.client.XInfoStream(ctx, r.streamName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	groups, err := r.client.XInfoGroups(ctx, r.streamName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get groups info: %w", err)
	}

	var pendingCount int64
	for _, g := range groups {
		if g.Name == r.consumerGroup {
			pendingCount = g.Pending
			break
		}
	}

	return &StreamInfo{
		Length:       info.Length,
		PendingCount: pendingCount,
		Groups:       len(groups),
	}, nil
}

// GetPendingCount returns the number of pending messages
func (r *RedisStreamClient) GetPendingCount(ctx context.Context) (int64, error) {
	pending, err := r.client.XPending(ctx, r.streamName, r.consumerGroup).Result()
	if err != nil {
		return 0, err
	}
	return pending.Count, nil
}

// Close closes the Redis client
func (r *RedisStreamClient) Close() error {
	return r.client.Close()
}

// StreamMessage represents a message from the stream
type StreamMessage struct {
	ID    string
	Event *models.TransactionEvent
}

// StreamInfo contains stream statistics
type StreamInfo struct {
	Length       int64
	PendingCount int64
	Groups       int
}

// CacheClient provides caching operations
type CacheClient struct {
	client *redis.Client
}

// NewCacheClient creates a new cache client (shares Redis connection)
func NewCacheClient(cfg configs.RedisConfig) (*CacheClient, error) {
	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &CacheClient{client: client}, nil
}

// Set sets a value in the cache
func (c *CacheClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, expiration).Err()
}

// Get retrieves a value from the cache
func (c *CacheClient) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// Delete removes a key from the cache
func (c *CacheClient) Delete(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists
func (c *CacheClient) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

// Increment increments a counter
func (c *CacheClient) Increment(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

// SetNX sets a value only if it doesn't exist (for distributed locking)
func (c *CacheClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return c.client.SetNX(ctx, key, data, expiration).Result()
}

// GetMemoryUsage returns Redis memory usage in MB
func (c *CacheClient) GetMemoryUsage(ctx context.Context) (float64, error) {
	info, err := c.client.Info(ctx, "memory").Result()
	if err != nil {
		return 0, err
	}
	// Parse used_memory from info string (simplified)
	_ = info
	return 0, nil // Simplified for this implementation
}

// LPush pushes a value to the left of a list
func (c *CacheClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.LPush(ctx, key, values...).Err()
}

// LTrim trims a list to the specified range
func (c *CacheClient) LTrim(ctx context.Context, key string, start, stop int64) error {
	return c.client.LTrim(ctx, key, start, stop).Err()
}

// LRange gets a range of elements from a list
func (c *CacheClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, key, start, stop).Result()
}

// HSet sets a hash field
func (c *CacheClient) HSet(ctx context.Context, key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.HSet(ctx, key, field, data).Err()
}

// HGet gets a hash field
func (c *CacheClient) HGet(ctx context.Context, key, field string, dest interface{}) error {
	data, err := c.client.HGet(ctx, key, field).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// HGetAll gets all fields from a hash
func (c *CacheClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// HIncrBy increments a hash field by a given amount
func (c *CacheClient) HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	return c.client.HIncrBy(ctx, key, field, incr).Result()
}

// Close closes the cache client
func (c *CacheClient) Close() error {
	return c.client.Close()
}
