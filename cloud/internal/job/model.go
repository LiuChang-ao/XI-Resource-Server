package job

import (
	"strconv"
	"time"
)

// Status represents the state of a job
type Status string

const (
	StatusPending   Status = "PENDING"
	StatusAssigned  Status = "ASSIGNED"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
	StatusCanceled  Status = "CANCELED"
	StatusLost      Status = "LOST"
)

// IsValid checks if the status is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusAssigned, StatusRunning, StatusSucceeded, StatusFailed, StatusCanceled, StatusLost:
		return true
	default:
		return false
	}
}

// CanTransitionTo checks if a status transition is valid
func (s Status) CanTransitionTo(target Status) bool {
	switch s {
	case StatusPending:
		return target == StatusAssigned || target == StatusCanceled
	case StatusAssigned:
		return target == StatusRunning || target == StatusCanceled || target == StatusLost
	case StatusRunning:
		return target == StatusSucceeded || target == StatusFailed || target == StatusCanceled || target == StatusLost
	case StatusSucceeded, StatusFailed, StatusCanceled, StatusLost:
		// Terminal states cannot transition
		return false
	default:
		return false
	}
}

// IsTerminal returns true if the status is a terminal state
func (s Status) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCanceled || s == StatusLost
}

// Job represents a compute job
type Job struct {
	JobID           string     `json:"job_id" db:"job_id"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	Status          Status     `json:"status" db:"status"`
	InputBucket     string     `json:"input_bucket" db:"input_bucket"`
	InputKey        string     `json:"input_key" db:"input_key"`
	OutputBucket    string     `json:"output_bucket" db:"output_bucket"`
	OutputKey       string     `json:"output_key" db:"output_key"`             // Can be empty if using prefix
	OutputPrefix    string     `json:"output_prefix" db:"output_prefix"`       // Prefix for output (e.g., "jobs/{job_id}/{attempt_id}/")
	OutputExtension string     `json:"output_extension" db:"output_extension"` // Output file extension (e.g., "json", "txt", "bin")
	AttemptID       int        `json:"attempt_id" db:"attempt_id"`
	AssignedAgentID string     `json:"assigned_agent_id" db:"assigned_agent_id"` // Optional, empty if not assigned
	LeaseID         string     `json:"lease_id" db:"lease_id"`                   // Optional, for future lease mechanism
	LeaseDeadline   *time.Time `json:"lease_deadline" db:"lease_deadline"`       // Optional, for future lease mechanism
	Command         string     `json:"command" db:"command"`                     // Command to execute on agent
	Stdout          string     `json:"stdout" db:"stdout"`                       // Command stdout output (truncated if too long)
	Stderr          string     `json:"stderr" db:"stderr"`                       // Command stderr output (truncated if too long)
}

// Validate validates the job fields
func (j *Job) Validate() error {
	if j.JobID == "" {
		return ErrInvalidJobID
	}
	if !j.Status.IsValid() {
		return ErrInvalidStatus
	}
	// Input is optional - jobs can run without input files
	// If input_bucket is provided, input_key must also be provided (and vice versa)
	if (j.InputBucket == "" && j.InputKey != "") || (j.InputBucket != "" && j.InputKey == "") {
		return ErrInvalidInput
	}
	// OutputBucket is optional - if empty, gateway will use OSS provider's default bucket
	// This allows jobs that only produce stdout/stderr without output files
	// Note: We still validate that if OutputBucket is provided, it should not be empty
	// (empty string is different from null/omitted in JSON)
	// Output must have either key or prefix (but both can be empty if command doesn't produce output file)
	// This is now allowed - jobs can succeed with only stdout/stderr
	if j.AttemptID < 1 {
		return ErrInvalidAttemptID
	}
	return nil
}

// GetOutputPath returns the output path (key or prefix-based)
func (j *Job) GetOutputPath() string {
	if j.OutputKey != "" {
		return j.OutputKey
	}
	return j.OutputPrefix
}

// EnsureOutputPrefix ensures the output prefix follows the pattern jobs/{job_id}/{attempt_id}/
// If a key is provided, it converts to prefix-based if the key matches the pattern
func (j *Job) EnsureOutputPrefix() {
	expectedPrefix := j.generateDefaultOutputPrefix()

	if j.OutputPrefix == "" && j.OutputKey == "" {
		// Generate default prefix
		j.OutputPrefix = expectedPrefix
	} else if j.OutputPrefix != "" {
		// Ensure it follows the pattern
		// If current prefix doesn't match, use default
		if j.OutputPrefix != expectedPrefix && !startsWith(j.OutputPrefix, expectedPrefix) {
			j.OutputPrefix = expectedPrefix
		}
	} else if j.OutputKey != "" {
		// If using key, check if it matches the pattern
		if startsWith(j.OutputKey, expectedPrefix) {
			// Key matches pattern, convert to prefix-based
			j.OutputPrefix = expectedPrefix
			j.OutputKey = ""
		} else {
			// Key doesn't match pattern, convert to prefix-based anyway (enforce pattern)
			j.OutputPrefix = expectedPrefix
			j.OutputKey = ""
		}
	}
}

// generateDefaultOutputPrefix generates the default output prefix
func (j *Job) generateDefaultOutputPrefix() string {
	return "jobs/" + j.JobID + "/" + intToString(j.AttemptID) + "/"
}

// generateDefaultOutputKey generates the default output key with extension
func (j *Job) generateDefaultOutputKey() string {
	extension := j.OutputExtension
	if extension == "" {
		extension = "bin" // Default extension
	}
	return j.generateDefaultOutputPrefix() + "output." + extension
}

// Helper functions
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func intToString(n int) string {
	// Simple conversion for attempt_id
	return strconv.Itoa(n)
}
