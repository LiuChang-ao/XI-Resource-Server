package job

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

// Store defines the interface for job persistence
type Store interface {
	// Create creates a new job
	Create(job *Job) error

	// Get retrieves a job by ID
	Get(jobID string) (*Job, error)

	// UpdateStatus updates the job status (with transition validation)
	UpdateStatus(jobID string, newStatus Status) error

	// UpdateAssignment updates the assigned agent and optionally lease info
	UpdateAssignment(jobID string, agentID string, leaseID string, leaseDeadline *time.Time) error

	// UpdateOutput updates the output key/prefix
	UpdateOutput(jobID string, outputKey, outputPrefix string) error

	// UpdateAttemptID updates the attempt_id for a job (used to ensure consistency)
	UpdateAttemptID(jobID string, attemptID int) error

	// UpdateStdoutStderr updates the stdout and stderr for a job
	UpdateStdoutStderr(jobID string, stdout, stderr string) error

	// UpdateMessage updates the message for a job
	UpdateMessage(jobID string, message string) error

	// List returns a list of jobs (with optional filters)
	List(limit int, offset int, status *Status) ([]*Job, error)

	// Close closes the database connection
	Close() error
}

// SQLiteStore implements Store using SQLite
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(dbPath string) (Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{db: db}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the jobs table if it doesn't exist
func (s *SQLiteStore) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS jobs (
		job_id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		status TEXT NOT NULL,
		input_bucket TEXT NOT NULL,
		input_key TEXT NOT NULL,
		output_bucket TEXT NOT NULL,
		output_key TEXT,
		output_prefix TEXT,
		output_extension TEXT,
		attempt_id INTEGER NOT NULL DEFAULT 1,
		assigned_agent_id TEXT,
		lease_id TEXT,
		lease_deadline DATETIME,
		command TEXT,
		job_type TEXT,
		forward_url TEXT,
		forward_method TEXT,
		forward_headers TEXT,
		forward_body TEXT,
		forward_timeout INTEGER,
		input_forward_mode TEXT,
		message TEXT,
		stdout TEXT,
		stderr TEXT,
		CHECK (attempt_id >= 1),
		CHECK (status IN ('PENDING', 'ASSIGNED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELED', 'LOST'))
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at);
	CREATE INDEX IF NOT EXISTS idx_jobs_assigned_agent ON jobs(assigned_agent_id);
	`

	_, err := s.db.Exec(query)
	if err != nil {
		return err
	}

	// Add new columns if they don't exist (for existing databases)
	// SQLite doesn't support IF NOT EXISTS for ALTER TABLE ADD COLUMN, so we'll ignore the error
	newColumns := []string{
		"command TEXT",
		"output_extension TEXT",
		"stdout TEXT",
		"stderr TEXT",
		"job_type TEXT",
		"forward_url TEXT",
		"forward_method TEXT",
		"forward_headers TEXT",
		"forward_body TEXT",
		"forward_timeout INTEGER",
		"input_forward_mode TEXT",
		"message TEXT",
	}
	for _, col := range newColumns {
		_, err = s.db.Exec(`ALTER TABLE jobs ADD COLUMN ` + col)
		// Ignore error if column already exists
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			// Only log warning, don't fail - column might already exist
		}
	}

	return nil
}

// Create creates a new job
func (s *SQLiteStore) Create(job *Job) error {
	if err := job.Validate(); err != nil {
		return err
	}

	// Ensure output prefix follows pattern
	job.EnsureOutputPrefix()

	query := `
	INSERT INTO jobs (
		job_id, created_at, status, input_bucket, input_key,
		output_bucket, output_key, output_prefix, output_extension, attempt_id,
		assigned_agent_id, lease_id, lease_deadline, command, job_type,
		forward_url, forward_method, forward_headers, forward_body, forward_timeout, input_forward_mode,
		message, stdout, stderr
	) VALUES (?, datetime(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Format time for SQLite
	createdAtStr := job.CreatedAt.Format(time.RFC3339)
	var leaseDeadlineStr interface{}
	if job.LeaseDeadline != nil {
		leaseDeadlineStr = job.LeaseDeadline.Format(time.RFC3339)
	}

	_, err := s.db.Exec(
		query,
		job.JobID,
		createdAtStr,
		string(job.Status),
		job.InputBucket,
		job.InputKey,
		job.OutputBucket,
		job.OutputKey,
		job.OutputPrefix,
		job.OutputExtension,
		job.AttemptID,
		job.AssignedAgentID,
		job.LeaseID,
		leaseDeadlineStr,
		job.Command,
		job.JobType,
		job.ForwardURL,
		job.ForwardMethod,
		job.ForwardHeaders,
		job.ForwardBody,
		job.ForwardTimeout,
		job.InputForward,
		job.Message,
		job.Stdout,
		job.Stderr,
	)

	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

// Get retrieves a job by ID
func (s *SQLiteStore) Get(jobID string) (*Job, error) {
	query := `
	SELECT 
		job_id, created_at, status, input_bucket, input_key,
		output_bucket, output_key, output_prefix, output_extension, attempt_id,
		assigned_agent_id, lease_id, lease_deadline, command, job_type,
		forward_url, forward_method, forward_headers, forward_body, forward_timeout, input_forward_mode,
		message, stdout, stderr
	FROM jobs
	WHERE job_id = ?
	`

	var job Job
	var createdAtStr string
	var statusStr string
	var leaseDeadline sql.NullTime
	var command sql.NullString
	var jobType sql.NullString
	var forwardURL sql.NullString
	var forwardMethod sql.NullString
	var forwardHeaders sql.NullString
	var forwardBody sql.NullString
	var forwardTimeout sql.NullInt64
	var inputForward sql.NullString
	var message sql.NullString
	var stdout sql.NullString
	var stderr sql.NullString
	var outputExtension sql.NullString

	err := s.db.QueryRow(query, jobID).Scan(
		&job.JobID,
		&createdAtStr,
		&statusStr,
		&job.InputBucket,
		&job.InputKey,
		&job.OutputBucket,
		&job.OutputKey,
		&job.OutputPrefix,
		&outputExtension,
		&job.AttemptID,
		&job.AssignedAgentID,
		&job.LeaseID,
		&leaseDeadline,
		&command,
		&jobType,
		&forwardURL,
		&forwardMethod,
		&forwardHeaders,
		&forwardBody,
		&forwardTimeout,
		&inputForward,
		&message,
		&stdout,
		&stderr,
	)

	if err == sql.ErrNoRows {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Parse timestamps - SQLite stores as RFC3339 or Unix timestamp
	job.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		// Try SQLite datetime format
		job.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse created_at: %w", err)
		}
	}

	job.Status = Status(statusStr)
	if !job.Status.IsValid() {
		return nil, fmt.Errorf("invalid status in database: %s", statusStr)
	}

	if leaseDeadline.Valid {
		job.LeaseDeadline = &leaseDeadline.Time
	}

	if command.Valid {
		job.Command = command.String
	} else {
		job.Command = ""
	}

	if jobType.Valid {
		job.JobType = JobType(jobType.String)
	} else {
		job.JobType = JobTypeCommand
	}

	if forwardURL.Valid {
		job.ForwardURL = forwardURL.String
	}
	if forwardMethod.Valid {
		job.ForwardMethod = forwardMethod.String
	}
	if forwardHeaders.Valid {
		job.ForwardHeaders = forwardHeaders.String
	}
	if forwardBody.Valid {
		job.ForwardBody = forwardBody.String
	}
	if forwardTimeout.Valid {
		job.ForwardTimeout = int(forwardTimeout.Int64)
	}
	if inputForward.Valid {
		job.InputForward = InputForwardMode(inputForward.String)
	}
	if message.Valid {
		job.Message = message.String
	} else {
		job.Message = ""
	}

	if outputExtension.Valid {
		job.OutputExtension = outputExtension.String
	} else {
		job.OutputExtension = "bin" // Default
	}

	if stdout.Valid {
		job.Stdout = stdout.String
	} else {
		job.Stdout = ""
	}

	if stderr.Valid {
		job.Stderr = stderr.String
	} else {
		job.Stderr = ""
	}

	return &job, nil
}

// UpdateStatus updates the job status (with transition validation)
func (s *SQLiteStore) UpdateStatus(jobID string, newStatus Status) error {
	if !newStatus.IsValid() {
		return ErrInvalidStatus
	}

	// Get current job to validate transition
	job, err := s.Get(jobID)
	if err != nil {
		return err
	}

	if !job.Status.CanTransitionTo(newStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, job.Status, newStatus)
	}

	query := `UPDATE jobs SET status = ? WHERE job_id = ?`
	_, err = s.db.Exec(query, string(newStatus), jobID)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// UpdateAssignment updates the assigned agent and optionally lease info
func (s *SQLiteStore) UpdateAssignment(jobID string, agentID string, leaseID string, leaseDeadline *time.Time) error {
	query := `UPDATE jobs SET assigned_agent_id = ?, lease_id = ?, lease_deadline = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, agentID, leaseID, leaseDeadline, jobID)
	if err != nil {
		return fmt.Errorf("failed to update assignment: %w", err)
	}
	return nil
}

// UpdateAttemptID updates the attempt_id for a job (used to ensure consistency)
func (s *SQLiteStore) UpdateAttemptID(jobID string, attemptID int) error {
	if attemptID < 1 {
		return fmt.Errorf("attempt_id must be >= 1")
	}
	query := `UPDATE jobs SET attempt_id = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, attemptID, jobID)
	if err != nil {
		return fmt.Errorf("failed to update attempt_id: %w", err)
	}
	return nil
}

// UpdateOutput updates the output key/prefix
func (s *SQLiteStore) UpdateOutput(jobID string, outputKey, outputPrefix string) error {
	query := `UPDATE jobs SET output_key = ?, output_prefix = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, outputKey, outputPrefix, jobID)
	if err != nil {
		return fmt.Errorf("failed to update output: %w", err)
	}
	return nil
}

// UpdateStdoutStderr updates the stdout and stderr for a job
func (s *SQLiteStore) UpdateStdoutStderr(jobID string, stdout, stderr string) error {
	query := `UPDATE jobs SET stdout = ?, stderr = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, stdout, stderr, jobID)
	if err != nil {
		return fmt.Errorf("failed to update stdout/stderr: %w", err)
	}
	return nil
}

// UpdateMessage updates the message for a job
func (s *SQLiteStore) UpdateMessage(jobID string, message string) error {
	query := `UPDATE jobs SET message = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, message, jobID)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}
	return nil
}

// List returns a list of jobs (with optional filters)
func (s *SQLiteStore) List(limit int, offset int, status *Status) ([]*Job, error) {
	query := `
	SELECT 
		job_id, created_at, status, input_bucket, input_key,
		output_bucket, output_key, output_prefix, output_extension, attempt_id,
		assigned_agent_id, lease_id, lease_deadline, command, job_type,
		forward_url, forward_method, forward_headers, forward_body, forward_timeout, input_forward_mode,
		message, stdout, stderr
	FROM jobs
	`
	args := []interface{}{}

	if status != nil {
		query += " WHERE status = ?"
		args = append(args, string(*status))
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var job Job
		var createdAtStr string
		var statusStr string
		var leaseDeadline sql.NullTime
		var command sql.NullString
		var jobType sql.NullString
		var forwardURL sql.NullString
		var forwardMethod sql.NullString
		var forwardHeaders sql.NullString
		var forwardBody sql.NullString
		var forwardTimeout sql.NullInt64
		var inputForward sql.NullString
		var message sql.NullString
		var stdout sql.NullString
		var stderr sql.NullString
		var outputExtension sql.NullString

		err := rows.Scan(
			&job.JobID,
			&createdAtStr,
			&statusStr,
			&job.InputBucket,
			&job.InputKey,
			&job.OutputBucket,
			&job.OutputKey,
			&job.OutputPrefix,
			&outputExtension,
			&job.AttemptID,
			&job.AssignedAgentID,
			&job.LeaseID,
			&leaseDeadline,
			&command,
			&jobType,
			&forwardURL,
			&forwardMethod,
			&forwardHeaders,
			&forwardBody,
			&forwardTimeout,
			&inputForward,
			&message,
			&stdout,
			&stderr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}

		// Parse timestamps - SQLite stores as RFC3339 or Unix timestamp
		job.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			// Try SQLite datetime format
			job.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse created_at: %w", err)
			}
		}

		job.Status = Status(statusStr)
		if !job.Status.IsValid() {
			return nil, fmt.Errorf("invalid status in database: %s", statusStr)
		}

		if leaseDeadline.Valid {
			job.LeaseDeadline = &leaseDeadline.Time
		}

		if command.Valid {
			job.Command = command.String
		} else {
			job.Command = ""
		}

		if jobType.Valid {
			job.JobType = JobType(jobType.String)
		} else {
			job.JobType = JobTypeCommand
		}
		if forwardURL.Valid {
			job.ForwardURL = forwardURL.String
		}
		if forwardMethod.Valid {
			job.ForwardMethod = forwardMethod.String
		}
		if forwardHeaders.Valid {
			job.ForwardHeaders = forwardHeaders.String
		}
		if forwardBody.Valid {
			job.ForwardBody = forwardBody.String
		}
		if forwardTimeout.Valid {
			job.ForwardTimeout = int(forwardTimeout.Int64)
		}
		if inputForward.Valid {
			job.InputForward = InputForwardMode(inputForward.String)
		}
		if message.Valid {
			job.Message = message.String
		} else {
			job.Message = ""
		}

		if outputExtension.Valid {
			job.OutputExtension = outputExtension.String
		} else {
			job.OutputExtension = "bin" // Default
		}

		if stdout.Valid {
			job.Stdout = stdout.String
		} else {
			job.Stdout = ""
		}

		if stderr.Valid {
			job.Stderr = stderr.String
		} else {
			job.Stderr = ""
		}

		jobs = append(jobs, &job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return jobs, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// MySQLStore implements Store using MySQL
type MySQLStore struct {
	db *sql.DB
}

// NewMySQLStore creates a new MySQL store
func NewMySQLStore(host string, port int, user, password, database, params string) (Store, error) {
	// Build DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, host, port, database)
	if params != "" {
		dsn += "?" + params
	} else {
		dsn += "?charset=utf8mb4&parseTime=True&loc=Local"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &MySQLStore{db: db}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the jobs table if it doesn't exist
func (s *MySQLStore) initSchema() error {
	// Create table first
	tableQuery := `
	CREATE TABLE IF NOT EXISTS jobs (
		job_id VARCHAR(255) PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		status VARCHAR(50) NOT NULL,
		input_bucket VARCHAR(255),
		input_key VARCHAR(512),
		output_bucket VARCHAR(255) NOT NULL,
		output_key VARCHAR(512),
		output_prefix VARCHAR(512),
		output_extension VARCHAR(50),
		attempt_id INT NOT NULL DEFAULT 1,
		assigned_agent_id VARCHAR(255),
		lease_id VARCHAR(255),
		lease_deadline DATETIME,
		command VARCHAR(8192),
		job_type VARCHAR(50),
		forward_url VARCHAR(2048),
		forward_method VARCHAR(20),
		forward_headers TEXT,
		forward_body LONGTEXT,
		forward_timeout INT,
		input_forward_mode VARCHAR(50),
		message TEXT,
		stdout TEXT,
		stderr TEXT,
		CHECK (attempt_id >= 1),
		CHECK (status IN ('PENDING', 'ASSIGNED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELED', 'LOST'))
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	if _, err := s.db.Exec(tableQuery); err != nil {
		return fmt.Errorf("failed to create jobs table: %w", err)
	}

	// Add new columns if they don't exist (for existing databases)
	// MySQL doesn't support IF NOT EXISTS for ALTER TABLE ADD COLUMN, so we'll check and ignore duplicate errors
	newColumns := []struct {
		name string
		typ  string
	}{
		{"command", "VARCHAR(8192)"},
		{"output_extension", "VARCHAR(50)"},
		{"stdout", "TEXT"},
		{"stderr", "TEXT"},
		{"job_type", "VARCHAR(50)"},
		{"forward_url", "VARCHAR(2048)"},
		{"forward_method", "VARCHAR(20)"},
		{"forward_headers", "TEXT"},
		{"forward_body", "LONGTEXT"},
		{"forward_timeout", "INT"},
		{"input_forward_mode", "VARCHAR(50)"},
		{"message", "TEXT"},
	}
	for _, col := range newColumns {
		_, err := s.db.Exec(fmt.Sprintf("ALTER TABLE jobs ADD COLUMN %s %s", col.name, col.typ))
		if err != nil {
			// Check if error is due to duplicate column
			errStr := strings.ToLower(err.Error())
			isDuplicate := strings.Contains(errStr, "duplicate column") ||
				strings.Contains(errStr, "already exists") ||
				strings.Contains(errStr, "1060") // MySQL error code for duplicate column
			if !isDuplicate {
				// Only return error if it's not a duplicate column error
				return fmt.Errorf("failed to add %s column: %w", col.name, err)
			}
			// Column already exists, continue silently
		}
	}

	// Create indexes separately (IF NOT EXISTS is only supported in MySQL 8.0.13+)
	// For compatibility with older MySQL versions, we'll try to create and ignore duplicate errors
	indexes := []struct {
		name string
		sql  string
	}{
		{"idx_jobs_status", "CREATE INDEX idx_jobs_status ON jobs(status)"},
		{"idx_jobs_created_at", "CREATE INDEX idx_jobs_created_at ON jobs(created_at)"},
		{"idx_jobs_assigned_agent", "CREATE INDEX idx_jobs_assigned_agent ON jobs(assigned_agent_id)"},
	}

	for _, idx := range indexes {
		// Try to create index, ignore error if it already exists
		_, err := s.db.Exec(idx.sql)
		if err != nil {
			// Check if error is due to duplicate key/index
			// MySQL error codes: 1061 = Duplicate key name, 42000 = general syntax error
			// Error messages may vary by MySQL version
			errStr := strings.ToLower(err.Error())
			isDuplicate := strings.Contains(errStr, "duplicate key name") ||
				strings.Contains(errStr, "already exists") ||
				strings.Contains(errStr, "duplicate") ||
				strings.Contains(errStr, "1061")

			if !isDuplicate {
				// Only return error if it's not a duplicate key error
				return fmt.Errorf("failed to create index %s: %w", idx.name, err)
			}
			// Index already exists, continue silently
		}
	}

	return nil
}

// Create creates a new job
func (s *MySQLStore) Create(job *Job) error {
	if err := job.Validate(); err != nil {
		return err
	}

	// Ensure output prefix follows pattern
	job.EnsureOutputPrefix()

	query := `
	INSERT INTO jobs (
		job_id, created_at, status, input_bucket, input_key,
		output_bucket, output_key, output_prefix, output_extension, attempt_id,
		assigned_agent_id, lease_id, lease_deadline, command, job_type,
		forward_url, forward_method, forward_headers, forward_body, forward_timeout, input_forward_mode,
		message, stdout, stderr
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(
		query,
		job.JobID,
		job.CreatedAt,
		string(job.Status),
		job.InputBucket,
		job.InputKey,
		job.OutputBucket,
		job.OutputKey,
		job.OutputPrefix,
		job.OutputExtension,
		job.AttemptID,
		job.AssignedAgentID,
		job.LeaseID,
		job.LeaseDeadline,
		job.Command,
		job.JobType,
		job.ForwardURL,
		job.ForwardMethod,
		job.ForwardHeaders,
		job.ForwardBody,
		job.ForwardTimeout,
		job.InputForward,
		job.Message,
		job.Stdout,
		job.Stderr,
	)

	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

// Get retrieves a job by ID
func (s *MySQLStore) Get(jobID string) (*Job, error) {
	query := `
	SELECT 
		job_id, created_at, status, input_bucket, input_key,
		output_bucket, output_key, output_prefix, output_extension, attempt_id,
		assigned_agent_id, lease_id, lease_deadline, command, job_type,
		forward_url, forward_method, forward_headers, forward_body, forward_timeout, input_forward_mode,
		message, stdout, stderr
	FROM jobs
	WHERE job_id = ?
	`

	var job Job
	var statusStr string
	var leaseDeadline sql.NullTime
	var command sql.NullString
	var jobType sql.NullString
	var forwardURL sql.NullString
	var forwardMethod sql.NullString
	var forwardHeaders sql.NullString
	var forwardBody sql.NullString
	var forwardTimeout sql.NullInt64
	var inputForward sql.NullString
	var message sql.NullString
	var stdout sql.NullString
	var stderr sql.NullString
	var outputExtension sql.NullString

	err := s.db.QueryRow(query, jobID).Scan(
		&job.JobID,
		&job.CreatedAt,
		&statusStr,
		&job.InputBucket,
		&job.InputKey,
		&job.OutputBucket,
		&job.OutputKey,
		&job.OutputPrefix,
		&outputExtension,
		&job.AttemptID,
		&job.AssignedAgentID,
		&job.LeaseID,
		&leaseDeadline,
		&command,
		&jobType,
		&forwardURL,
		&forwardMethod,
		&forwardHeaders,
		&forwardBody,
		&forwardTimeout,
		&inputForward,
		&message,
		&stdout,
		&stderr,
	)

	if err == sql.ErrNoRows {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	job.Status = Status(statusStr)
	if !job.Status.IsValid() {
		return nil, fmt.Errorf("invalid status in database: %s", statusStr)
	}

	if leaseDeadline.Valid {
		job.LeaseDeadline = &leaseDeadline.Time
	}

	if command.Valid {
		job.Command = command.String
	} else {
		job.Command = ""
	}

	if jobType.Valid {
		job.JobType = JobType(jobType.String)
	} else {
		job.JobType = JobTypeCommand
	}
	if forwardURL.Valid {
		job.ForwardURL = forwardURL.String
	}
	if forwardMethod.Valid {
		job.ForwardMethod = forwardMethod.String
	}
	if forwardHeaders.Valid {
		job.ForwardHeaders = forwardHeaders.String
	}
	if forwardBody.Valid {
		job.ForwardBody = forwardBody.String
	}
	if forwardTimeout.Valid {
		job.ForwardTimeout = int(forwardTimeout.Int64)
	}
	if inputForward.Valid {
		job.InputForward = InputForwardMode(inputForward.String)
	}
	if message.Valid {
		job.Message = message.String
	} else {
		job.Message = ""
	}

	if outputExtension.Valid {
		job.OutputExtension = outputExtension.String
	} else {
		job.OutputExtension = "bin" // Default
	}

	if stdout.Valid {
		job.Stdout = stdout.String
	} else {
		job.Stdout = ""
	}

	if stderr.Valid {
		job.Stderr = stderr.String
	} else {
		job.Stderr = ""
	}

	return &job, nil
}

// UpdateStatus updates the job status (with transition validation)
func (s *MySQLStore) UpdateStatus(jobID string, newStatus Status) error {
	if !newStatus.IsValid() {
		return ErrInvalidStatus
	}

	// Get current job to validate transition
	job, err := s.Get(jobID)
	if err != nil {
		return err
	}

	if !job.Status.CanTransitionTo(newStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, job.Status, newStatus)
	}

	query := `UPDATE jobs SET status = ? WHERE job_id = ?`
	_, err = s.db.Exec(query, string(newStatus), jobID)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// UpdateAssignment updates the assigned agent and optionally lease info
func (s *MySQLStore) UpdateAssignment(jobID string, agentID string, leaseID string, leaseDeadline *time.Time) error {
	query := `UPDATE jobs SET assigned_agent_id = ?, lease_id = ?, lease_deadline = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, agentID, leaseID, leaseDeadline, jobID)
	if err != nil {
		return fmt.Errorf("failed to update assignment: %w", err)
	}
	return nil
}

// UpdateAttemptID updates the attempt_id for a job (used to ensure consistency)
func (s *MySQLStore) UpdateAttemptID(jobID string, attemptID int) error {
	if attemptID < 1 {
		return fmt.Errorf("attempt_id must be >= 1")
	}
	query := `UPDATE jobs SET attempt_id = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, attemptID, jobID)
	if err != nil {
		return fmt.Errorf("failed to update attempt_id: %w", err)
	}
	return nil
}

// UpdateOutput updates the output key/prefix
func (s *MySQLStore) UpdateOutput(jobID string, outputKey, outputPrefix string) error {
	query := `UPDATE jobs SET output_key = ?, output_prefix = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, outputKey, outputPrefix, jobID)
	if err != nil {
		return fmt.Errorf("failed to update output: %w", err)
	}
	return nil
}

// UpdateStdoutStderr updates the stdout and stderr for a job
func (s *MySQLStore) UpdateStdoutStderr(jobID string, stdout, stderr string) error {
	query := `UPDATE jobs SET stdout = ?, stderr = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, stdout, stderr, jobID)
	if err != nil {
		return fmt.Errorf("failed to update stdout/stderr: %w", err)
	}
	return nil
}

// UpdateMessage updates the message for a job
func (s *MySQLStore) UpdateMessage(jobID string, message string) error {
	query := `UPDATE jobs SET message = ? WHERE job_id = ?`
	_, err := s.db.Exec(query, message, jobID)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}
	return nil
}

// List returns a list of jobs (with optional filters)
func (s *MySQLStore) List(limit int, offset int, status *Status) ([]*Job, error) {
	query := `
	SELECT 
		job_id, created_at, status, input_bucket, input_key,
		output_bucket, output_key, output_prefix, output_extension, attempt_id,
		assigned_agent_id, lease_id, lease_deadline, command, job_type,
		forward_url, forward_method, forward_headers, forward_body, forward_timeout, input_forward_mode,
		message, stdout, stderr
	FROM jobs
	`
	args := []interface{}{}

	if status != nil {
		query += " WHERE status = ?"
		args = append(args, string(*status))
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var job Job
		var statusStr string
		var leaseDeadline sql.NullTime
		var command sql.NullString
		var jobType sql.NullString
		var forwardURL sql.NullString
		var forwardMethod sql.NullString
		var forwardHeaders sql.NullString
		var forwardBody sql.NullString
		var forwardTimeout sql.NullInt64
		var inputForward sql.NullString
		var message sql.NullString
		var stdout sql.NullString
		var stderr sql.NullString
		var outputExtension sql.NullString

		err := rows.Scan(
			&job.JobID,
			&job.CreatedAt,
			&statusStr,
			&job.InputBucket,
			&job.InputKey,
			&job.OutputBucket,
			&job.OutputKey,
			&job.OutputPrefix,
			&outputExtension,
			&job.AttemptID,
			&job.AssignedAgentID,
			&job.LeaseID,
			&leaseDeadline,
			&command,
			&jobType,
			&forwardURL,
			&forwardMethod,
			&forwardHeaders,
			&forwardBody,
			&forwardTimeout,
			&inputForward,
			&message,
			&stdout,
			&stderr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}

		job.Status = Status(statusStr)
		if !job.Status.IsValid() {
			return nil, fmt.Errorf("invalid status in database: %s", statusStr)
		}

		if leaseDeadline.Valid {
			job.LeaseDeadline = &leaseDeadline.Time
		}

		if command.Valid {
			job.Command = command.String
		} else {
			job.Command = ""
		}

		if jobType.Valid {
			job.JobType = JobType(jobType.String)
		} else {
			job.JobType = JobTypeCommand
		}
		if forwardURL.Valid {
			job.ForwardURL = forwardURL.String
		}
		if forwardMethod.Valid {
			job.ForwardMethod = forwardMethod.String
		}
		if forwardHeaders.Valid {
			job.ForwardHeaders = forwardHeaders.String
		}
		if forwardBody.Valid {
			job.ForwardBody = forwardBody.String
		}
		if forwardTimeout.Valid {
			job.ForwardTimeout = int(forwardTimeout.Int64)
		}
		if inputForward.Valid {
			job.InputForward = InputForwardMode(inputForward.String)
		}
		if message.Valid {
			job.Message = message.String
		} else {
			job.Message = ""
		}

		if outputExtension.Valid {
			job.OutputExtension = outputExtension.String
		} else {
			job.OutputExtension = "bin" // Default
		}

		if stdout.Valid {
			job.Stdout = stdout.String
		} else {
			job.Stdout = ""
		}

		if stderr.Valid {
			job.Stderr = stderr.String
		} else {
			job.Stderr = ""
		}

		jobs = append(jobs, &job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return jobs, nil
}

// Close closes the database connection
func (s *MySQLStore) Close() error {
	return s.db.Close()
}

// NewStore creates a new store based on the provided configuration
// If MySQL is configured, it uses MySQL; otherwise, it falls back to SQLite
func NewStore(cfg *DBConfig) (Store, error) {
	if cfg.IsMySQLConfigured() {
		return NewMySQLStore(
			cfg.MySQLHost,
			cfg.MySQLPort,
			cfg.MySQLUser,
			cfg.MySQLPassword,
			cfg.MySQLDatabase,
			cfg.MySQLParams,
		)
	}
	// Fall back to SQLite
	return NewSQLiteStore(cfg.SQLitePath)
}
