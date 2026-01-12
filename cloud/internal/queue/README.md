# Redis Queue Package

This package provides Redis-based job queue functionality for the cloud server.

## Configuration

Redis configuration is loaded from environment variables (via `.env` file or system environment).

### Environment Variables

#### Option 1: Redis URL (Recommended)
```
REDIS_URL=redis://localhost:6379/0
# Or with authentication:
REDIS_URL=redis://username:password@localhost:6379/0
# Or with TLS:
REDIS_URL=rediss://username:password@localhost:6379/0
```

#### Option 2: Individual Configuration
```
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=your_password
REDIS_DATABASE=0
REDIS_USERNAME=your_username
REDIS_TLS_ENABLED=false
```

### .env File Example

Create a `.env` file in the `cloud/` directory or project root:

```env
# Redis Configuration (for job queue)
REDIS_URL=redis://localhost:6379/0

# Or use individual config:
# REDIS_HOST=localhost
# REDIS_PORT=6379
# REDIS_PASSWORD=
# REDIS_DATABASE=0
```

**Note:** The `.env` file is automatically ignored by `.gitignore` to prevent committing sensitive credentials.

## Usage

The queue is automatically initialized in `main.go` if Redis is configured. If Redis is not available or not configured, jobs will still be created in the database but will not be enqueued (the server will log a warning).

## Queue Operations

- `Enqueue(ctx, jobID)` - Adds a job to the queue (FIFO)
- `Dequeue(ctx)` - Removes and returns a job ID (non-blocking)
- `DequeueBlocking(ctx, timeoutSec)` - Removes and returns a job ID (blocking)
- `Peek(ctx)` - Returns a job ID without removing it
- `Size(ctx)` - Returns the number of jobs in the queue
- `Remove(ctx, jobID)` - Removes a specific job ID from the queue

## Default Queue Key

The default Redis key for the job queue is `jobs:pending`. This can be customized using `NewRedisQueueWithKey()`.
