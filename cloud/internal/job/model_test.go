package job

import (
	"strconv"
	"testing"
	"time"
)

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		status Status
		valid  bool
	}{
		{StatusPending, true},
		{StatusAssigned, true},
		{StatusRunning, true},
		{StatusSucceeded, true},
		{StatusFailed, true},
		{StatusCanceled, true},
		{StatusLost, true},
		{Status("INVALID"), false},
		{Status(""), false},
	}

	for _, tt := range tests {
		name := string(tt.status)
		if name == "" {
			name = "empty_string"
		}
		t.Run(name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.valid {
				t.Errorf("Status.IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from    Status
		to      Status
		allowed bool
	}{
		// PENDING transitions
		{StatusPending, StatusAssigned, true},
		{StatusPending, StatusCanceled, true},
		{StatusPending, StatusRunning, false},
		{StatusPending, StatusSucceeded, false},

		// ASSIGNED transitions
		{StatusAssigned, StatusRunning, true},
		{StatusAssigned, StatusCanceled, true},
		{StatusAssigned, StatusLost, true},
		{StatusAssigned, StatusSucceeded, false},
		{StatusAssigned, StatusPending, false},

		// RUNNING transitions
		{StatusRunning, StatusSucceeded, true},
		{StatusRunning, StatusFailed, true},
		{StatusRunning, StatusCanceled, true},
		{StatusRunning, StatusLost, true},
		{StatusRunning, StatusPending, false},
		{StatusRunning, StatusAssigned, false},

		// Terminal states cannot transition
		{StatusSucceeded, StatusRunning, false},
		{StatusFailed, StatusRunning, false},
		{StatusCanceled, StatusRunning, false},
		{StatusLost, StatusRunning, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"_to_"+string(tt.to), func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.allowed {
				t.Errorf("Status.CanTransitionTo() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   Status
		terminal bool
	}{
		{StatusPending, false},
		{StatusAssigned, false},
		{StatusRunning, false},
		{StatusSucceeded, true},
		{StatusFailed, true},
		{StatusCanceled, true},
		{StatusLost, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.terminal {
				t.Errorf("Status.IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

func TestJob_Validate(t *testing.T) {
	tests := []struct {
		name    string
		job     *Job
		wantErr bool
	}{
		{
			name: "valid job",
			job: &Job{
				JobID:        "test-job-id",
				CreatedAt:    time.Now(),
				Status:       StatusPending,
				InputBucket:  "input-bucket",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
			},
			wantErr: false,
		},
		{
			name: "missing job_id",
			job: &Job{
				JobID:        "",
				InputBucket:  "input-bucket",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			job: &Job{
				JobID:        "test-job-id",
				Status:       Status("INVALID"),
				InputBucket:  "input-bucket",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
			},
			wantErr: true,
		},
		{
			name: "missing input bucket but has input key (invalid)",
			job: &Job{
				JobID:        "test-job-id",
				InputBucket:  "",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
			},
			wantErr: true,
		},
		{
			name: "missing input key but has input bucket (invalid)",
			job: &Job{
				JobID:        "test-job-id",
				InputBucket:  "input-bucket",
				InputKey:     "",
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
			},
			wantErr: true,
		},
		{
			name: "no input file (both empty - valid)",
			job: &Job{
				JobID:        "test-job-id",
				Status:       StatusPending,
				InputBucket:  "",
				InputKey:     "",
				OutputBucket: "output-bucket",
				OutputPrefix: "jobs/test-job-id/1/",
				AttemptID:    1,
			},
			wantErr: false,
		},
		{
			name: "missing output bucket",
			job: &Job{
				JobID:       "test-job-id",
				InputBucket: "input-bucket",
				InputKey:    "input-key",
				OutputKey:   "output-key",
				AttemptID:   1,
			},
			wantErr: true,
		},
		{
			name: "missing both output key and prefix",
			job: &Job{
				JobID:        "test-job-id",
				InputBucket:  "input-bucket",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				AttemptID:    1,
			},
			wantErr: true,
		},
		{
			name: "invalid attempt_id",
			job: &Job{
				JobID:        "test-job-id",
				InputBucket:  "input-bucket",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    0,
			},
			wantErr: true,
		},
		{
			name: "valid forward job",
			job: &Job{
				JobID:        "forward-job-1",
				CreatedAt:    time.Now(),
				Status:       StatusPending,
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
				JobType:      JobTypeForwardHTTP,
				ForwardURL:   "http://127.0.0.1:8080/api",
				InputForward: InputForwardModeURL,
			},
			wantErr: false,
		},
		{
			name: "forward job missing forward_url",
			job: &Job{
				JobID:        "forward-job-2",
				CreatedAt:    time.Now(),
				Status:       StatusPending,
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
				JobType:      JobTypeForwardHTTP,
			},
			wantErr: true,
		},
		{
			name: "invalid job type",
			job: &Job{
				JobID:        "invalid-type-job",
				CreatedAt:    time.Now(),
				Status:       StatusPending,
				OutputBucket: "output-bucket",
				OutputKey:    "output-key",
				AttemptID:    1,
				JobType:      JobType("INVALID"),
			},
			wantErr: true,
		},
		{
			name: "valid with output prefix",
			job: &Job{
				JobID:        "test-job-id",
				Status:       StatusPending,
				InputBucket:  "input-bucket",
				InputKey:     "input-key",
				OutputBucket: "output-bucket",
				OutputPrefix: "jobs/test-job-id/1/",
				AttemptID:    1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Job.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJob_EnsureOutputPrefix(t *testing.T) {
	tests := []struct {
		name           string
		job            *Job
		expectedPrefix string
	}{
		{
			name: "empty prefix and key - generates default",
			job: &Job{
				JobID:     "test-job-123",
				AttemptID: 1,
			},
			expectedPrefix: "jobs/test-job-123/1/",
		},
		{
			name: "existing prefix - keeps it",
			job: &Job{
				JobID:        "test-job-123",
				AttemptID:    1,
				OutputPrefix: "jobs/test-job-123/1/",
			},
			expectedPrefix: "jobs/test-job-123/1/",
		},
		{
			name: "key that matches pattern - converts to prefix",
			job: &Job{
				JobID:     "test-job-123",
				AttemptID: 1,
				OutputKey: "jobs/test-job-123/1/output.zip",
			},
			expectedPrefix: "jobs/test-job-123/1/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.job.EnsureOutputPrefix()
			if tt.job.OutputPrefix != tt.expectedPrefix {
				t.Errorf("Job.EnsureOutputPrefix() prefix = %v, want %v", tt.job.OutputPrefix, tt.expectedPrefix)
			}
		})
	}
}

func TestJob_OutputPathConsistency(t *testing.T) {
	tests := []struct {
		name        string
		job         *Job
		wantErr     bool
		description string
	}{
		{
			name: "valid prefix matches job_id and attempt_id",
			job: &Job{
				JobID:        "job-abc-123",
				AttemptID:    1,
				OutputPrefix: "jobs/job-abc-123/1/",
			},
			wantErr:     false,
			description: "Prefix correctly matches job_id and attempt_id",
		},
		{
			name: "invalid prefix - wrong job_id",
			job: &Job{
				JobID:        "job-abc-123",
				AttemptID:    1,
				OutputPrefix: "jobs/wrong-job-id/1/",
			},
			wantErr:     true,
			description: "Prefix with wrong job_id should be rejected",
		},
		{
			name: "invalid prefix - wrong attempt_id",
			job: &Job{
				JobID:        "job-abc-123",
				AttemptID:    1,
				OutputPrefix: "jobs/job-abc-123/2/",
			},
			wantErr:     true,
			description: "Prefix with wrong attempt_id should be rejected",
		},
		{
			name: "invalid key - wrong job_id",
			job: &Job{
				JobID:     "job-abc-123",
				AttemptID: 1,
				OutputKey: "jobs/wrong-job-id/1/output.zip",
			},
			wantErr:     true,
			description: "Key with wrong job_id should be rejected",
		},
		{
			name: "invalid key - wrong attempt_id",
			job: &Job{
				JobID:     "job-abc-123",
				AttemptID: 1,
				OutputKey: "jobs/job-abc-123/2/output.zip",
			},
			wantErr:     true,
			description: "Key with wrong attempt_id should be rejected",
		},
		{
			name: "valid key matches job_id and attempt_id",
			job: &Job{
				JobID:     "job-abc-123",
				AttemptID: 1,
				OutputKey: "jobs/job-abc-123/1/output.zip",
			},
			wantErr:     false,
			description: "Key correctly matches job_id and attempt_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// EnsureOutputPrefix should enforce consistency
			tt.job.EnsureOutputPrefix()

			// After EnsureOutputPrefix, the output should match the pattern
			expectedPrefix := "jobs/" + tt.job.JobID + "/" + strconv.Itoa(tt.job.AttemptID) + "/"

			if tt.wantErr {
				// If we expect an error, the prefix should have been corrected
				if tt.job.OutputPrefix != expectedPrefix {
					t.Errorf("Expected prefix to be corrected to %v, got %v", expectedPrefix, tt.job.OutputPrefix)
				}
				// OutputKey should be cleared if it was invalid
				if tt.job.OutputKey != "" && !startsWith(tt.job.OutputKey, expectedPrefix) {
					// Key should have been cleared or converted
					if tt.job.OutputKey != "" {
						t.Errorf("Invalid OutputKey should be cleared or converted, got %v", tt.job.OutputKey)
					}
				}
			} else {
				// Valid cases should preserve or set correct prefix
				if tt.job.OutputPrefix != expectedPrefix {
					t.Errorf("OutputPrefix = %v, want %v", tt.job.OutputPrefix, expectedPrefix)
				}
				// If using key, it should match the prefix
				if tt.job.OutputKey != "" && !startsWith(tt.job.OutputKey, expectedPrefix) {
					t.Errorf("OutputKey = %v, should start with prefix %v", tt.job.OutputKey, expectedPrefix)
				}
			}
		})
	}
}
