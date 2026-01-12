package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrQueueEmpty    = errors.New("queue is empty")
	ErrJobNotInQueue = errors.New("job not in queue")
)

const (
	// DefaultQueueKey is the Redis key for the job queue
	DefaultQueueKey = "jobs:pending"
)

// Queue manages the Redis queue for job scheduling
type Queue interface {
	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, jobID string) error

	// Dequeue removes and returns a job ID from the queue (blocking)
	Dequeue(ctx context.Context) (string, error)

	// Peek returns a job ID from the queue without removing it
	Peek(ctx context.Context) (string, error)

	// Size returns the number of jobs in the queue
	Size(ctx context.Context) (int64, error)

	// Remove removes a specific job ID from the queue
	Remove(ctx context.Context, jobID string) error
}

// RedisQueue implements Queue using Redis
type RedisQueue struct {
	client *redis.Client
	key    string
}

// NewRedisQueue creates a new Redis queue
func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{
		client: client,
		key:    DefaultQueueKey,
	}
}

// NewRedisQueueWithKey creates a new Redis queue with a custom key
func NewRedisQueueWithKey(client *redis.Client, key string) *RedisQueue {
	return &RedisQueue{
		client: client,
		key:    key,
	}
}

// Enqueue adds a job ID to the queue (using LPUSH for FIFO with RPOP)
// We use LPUSH to add to the left, and RPOP to remove from the right (FIFO)
func (q *RedisQueue) Enqueue(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("job_id cannot be empty")
	}

	// Serialize job ID as JSON string for consistency (though it's already a string)
	// This allows for future extensibility if we need to add metadata
	jobData, err := json.Marshal(jobID)
	if err != nil {
		return fmt.Errorf("failed to marshal job ID: %w", err)
	}

	// LPUSH adds to the left (head) of the list
	if err := q.client.LPush(ctx, q.key, jobData).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	return nil
}

// Dequeue removes and returns a job ID from the queue (blocking, FIFO)
// Uses RPOP to remove from the right (tail), making it FIFO with LPUSH
func (q *RedisQueue) Dequeue(ctx context.Context) (string, error) {
	// RPOP removes from the right (tail) - blocking with timeout
	// Using BRPOP with 0 timeout for blocking, but we'll use RPOP for non-blocking
	// For blocking behavior, we'd use BRPOP with a timeout
	result, err := q.client.RPop(ctx, q.key).Result()
	if err == redis.Nil {
		return "", ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("failed to dequeue job: %w", err)
	}

	var jobID string
	if err := json.Unmarshal([]byte(result), &jobID); err != nil {
		// If unmarshal fails, try treating as plain string (backward compatibility)
		jobID = result
	}

	return jobID, nil
}

// DequeueBlocking removes and returns a job ID from the queue (blocking with timeout)
func (q *RedisQueue) DequeueBlocking(ctx context.Context, timeoutSec int) (string, error) {
	// BRPOP blocks until an element is available or timeout
	result, err := q.client.BRPop(ctx, time.Duration(timeoutSec)*time.Second, q.key).Result()
	if err == redis.Nil {
		return "", ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("failed to dequeue job: %w", err)
	}

	if len(result) < 2 {
		return "", fmt.Errorf("invalid result from BRPOP: %v", result)
	}

	// BRPOP returns [key, value]
	jobData := result[1]
	var jobID string
	if err := json.Unmarshal([]byte(jobData), &jobID); err != nil {
		// If unmarshal fails, try treating as plain string
		jobID = jobData
	}

	return jobID, nil
}

// Peek returns a job ID from the queue without removing it
func (q *RedisQueue) Peek(ctx context.Context) (string, error) {
	// LINDEX gets element at index -1 (rightmost/tail, which is next to dequeue)
	result, err := q.client.LIndex(ctx, q.key, -1).Result()
	if err == redis.Nil {
		return "", ErrQueueEmpty
	}
	if err != nil {
		return "", fmt.Errorf("failed to peek queue: %w", err)
	}

	var jobID string
	if err := json.Unmarshal([]byte(result), &jobID); err != nil {
		jobID = result
	}

	return jobID, nil
}

// Size returns the number of jobs in the queue
func (q *RedisQueue) Size(ctx context.Context) (int64, error) {
	size, err := q.client.LLen(ctx, q.key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue size: %w", err)
	}
	return size, nil
}

// Remove removes a specific job ID from the queue
func (q *RedisQueue) Remove(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("job_id cannot be empty")
	}

	// Serialize job ID to match what we stored
	jobData, err := json.Marshal(jobID)
	if err != nil {
		return fmt.Errorf("failed to marshal job ID: %w", err)
	}

	// LREM removes matching elements (count=0 removes all matches)
	removed, err := q.client.LRem(ctx, q.key, 0, jobData).Result()
	if err != nil {
		return fmt.Errorf("failed to remove job from queue: %w", err)
	}

	if removed == 0 {
		return ErrJobNotInQueue
	}

	return nil
}
