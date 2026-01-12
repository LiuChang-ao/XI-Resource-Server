# Job Management

This package implements the job data model and state machine for the cloud server.

## Features

- **Job Model**: Complete job data structure with all required fields
- **State Machine**: Validated state transitions (PENDING → ASSIGNED → RUNNING → SUCCEEDED/FAILED/CANCELED)
- **Persistence**: SQLite or MySQL-based storage with full CRUD operations
- **Output Path Validation**: Ensures output paths follow `jobs/{job_id}/{attempt_id}/` pattern

## Job Status Flow

```
PENDING → ASSIGNED → RUNNING → SUCCEEDED
                              → FAILED
                              → CANCELED
                              → LOST
```

## API Endpoints

- `POST /api/jobs` - Create a new job
- `GET /api/jobs/{job_id}` - Get job details
- `GET /api/jobs` - List jobs (with optional status filter)

## Database Configuration

The job store supports both SQLite (default) and MySQL. Configuration is loaded from environment variables or `.env` file.

### SQLite (Default)

If no MySQL configuration is provided, the system uses SQLite. You can optionally set:
- `SQLITE_PATH`: Path to SQLite database file (default: `jobs.db`)

### MySQL

To use MySQL, set the following environment variables in `.env` or system environment:
- `DB_TYPE=mysql`
- `MYSQL_HOST`: MySQL host (required)
- `MYSQL_PORT`: MySQL port (default: 3306)
- `MYSQL_USER`: MySQL username (required)
- `MYSQL_PASSWORD`: MySQL password (required)
- `MYSQL_DATABASE`: MySQL database name (required)
- `MYSQL_PARAMS`: Additional connection parameters (optional, default: `charset=utf8mb4&parseTime=True&loc=Local`)

Example `.env` configuration:
```env
# Database configuration
DB_TYPE=mysql
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=your_password
MYSQL_DATABASE=xiresource
MYSQL_PARAMS=charset=utf8mb4&parseTime=True&loc=Local
```

## Database Schema

The database (SQLite or MySQL) includes:
- `job_id` (PRIMARY KEY)
- `created_at` (DATETIME)
- `status` (TEXT, CHECK constraint)
- `input_bucket`, `input_key`
- `output_bucket`, `output_key`, `output_prefix`
- `attempt_id` (INTEGER, >= 1)
- `assigned_agent_id` (optional)
- `lease_id`, `lease_deadline` (optional, for future use)

## Usage Example

### Using Configuration (Recommended)

```go
// Load configuration from environment
dbConfig, err := job.LoadConfigFromEnv()
if err != nil {
    log.Fatalf("Failed to load database config: %v", err)
}

// Create store based on configuration
store, err := job.NewStore(dbConfig)
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

### Direct SQLite Store (for testing)

```go
// Create SQLite store directly (for testing or when you know you want SQLite)
store, err := job.NewSQLiteStore("jobs.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

// Create job
newJob := &job.Job{
    JobID:        uuid.New().String(),
    CreatedAt:    time.Now(),
    Status:        job.StatusPending,
    InputBucket:   "input-bucket",
    InputKey:      "input-key",
    OutputBucket:  "output-bucket",
    AttemptID:     1,
}
newJob.EnsureOutputPrefix() // Ensures output follows pattern
err = store.Create(newJob)

// Update status (with validation)
err = store.UpdateStatus(jobID, job.StatusAssigned)
```

## Testing

Run tests with:
```bash
go test ./internal/job -v
```

Note: SQLite tests require CGO to be enabled. On Windows, you may need to install a C compiler (e.g., via MinGW or TDM-GCC).
