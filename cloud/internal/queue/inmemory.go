package queue

import (
	"context"
	"errors"
	"sync"
)

// InMemoryQueue implements Queue using an in-memory slice (for testing)
type InMemoryQueue struct {
	mu    sync.Mutex
	items []string
}

// NewInMemoryQueue creates a new in-memory queue
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		items: make([]string, 0),
	}
}

// Enqueue adds a job ID to the queue
func (q *InMemoryQueue) Enqueue(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.New("job_id cannot be empty")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = append(q.items, jobID)
	return nil
}

// Dequeue removes and returns a job ID from the queue (FIFO)
func (q *InMemoryQueue) Dequeue(ctx context.Context) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return "", ErrQueueEmpty
	}

	jobID := q.items[0]
	q.items = q.items[1:]
	return jobID, nil
}

// Peek returns a job ID from the queue without removing it
func (q *InMemoryQueue) Peek(ctx context.Context) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return "", ErrQueueEmpty
	}

	return q.items[0], nil
}

// Size returns the number of jobs in the queue
func (q *InMemoryQueue) Size(ctx context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	return int64(len(q.items)), nil
}

// Remove removes a specific job ID from the queue
func (q *InMemoryQueue) Remove(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.New("job_id cannot be empty")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for i, item := range q.items {
		if item == jobID {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return nil
		}
	}

	return ErrJobNotInQueue
}

// Clear clears all items from the queue (for testing only)
func (q *InMemoryQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = make([]string, 0)
}
