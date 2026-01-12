package job

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) Store {
	// Create a temporary database file
	tmpFile, err := os.CreateTemp("", "test_jobs_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Cleanup function
	t.Cleanup(func() {
		store.Close()
		os.Remove(tmpFile.Name())
	})

	return store
}

func TestStore_CreateAndGet(t *testing.T) {
	store := setupTestStore(t)

	job := &Job{
		JobID:           "test-job-123",
		CreatedAt:       time.Now(),
		Status:          StatusPending,
		InputBucket:     "input-bucket",
		InputKey:        "input-key",
		OutputBucket:    "output-bucket",
		OutputKey:       "output-key",
		OutputPrefix:    "",
		AttemptID:       1,
		AssignedAgentID: "",
		LeaseID:         "",
		LeaseDeadline:   nil,
	}

	// Create job
	if err := store.Create(job); err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Get job
	retrieved, err := store.Get("test-job-123")
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	// Verify fields
	if retrieved.JobID != job.JobID {
		t.Errorf("JobID = %v, want %v", retrieved.JobID, job.JobID)
	}
	if retrieved.Status != job.Status {
		t.Errorf("Status = %v, want %v", retrieved.Status, job.Status)
	}
	if retrieved.InputBucket != job.InputBucket {
		t.Errorf("InputBucket = %v, want %v", retrieved.InputBucket, job.InputBucket)
	}
	if retrieved.InputKey != job.InputKey {
		t.Errorf("InputKey = %v, want %v", retrieved.InputKey, job.InputKey)
	}
	if retrieved.OutputBucket != job.OutputBucket {
		t.Errorf("OutputBucket = %v, want %v", retrieved.OutputBucket, job.OutputBucket)
	}
	if retrieved.AttemptID != job.AttemptID {
		t.Errorf("AttemptID = %v, want %v", retrieved.AttemptID, job.AttemptID)
	}

	// Verify output path consistency: EnsureOutputPrefix should have been called
	// Use strconv directly since intToString is not exported
	expectedPrefix := "jobs/" + job.JobID + "/" + strconv.Itoa(job.AttemptID) + "/"
	if retrieved.OutputPrefix != expectedPrefix {
		t.Errorf("OutputPrefix = %v, want %v (should match jobs/{job_id}/{attempt_id}/)", retrieved.OutputPrefix, expectedPrefix)
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	store := setupTestStore(t)

	job := &Job{
		JobID:        "test-job-123",
		CreatedAt:    time.Now(),
		Status:       StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "input-key",
		OutputBucket: "output-bucket",
		OutputKey:    "output-key",
		AttemptID:    1,
	}

	// Create job
	if err := store.Create(job); err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Valid transition: PENDING -> ASSIGNED
	if err := store.UpdateStatus("test-job-123", StatusAssigned); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	retrieved, err := store.Get("test-job-123")
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}
	if retrieved.Status != StatusAssigned {
		t.Errorf("Status = %v, want %v", retrieved.Status, StatusAssigned)
	}

	// Valid transition: ASSIGNED -> RUNNING
	if err := store.UpdateStatus("test-job-123", StatusRunning); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Invalid transition: RUNNING -> PENDING
	if err := store.UpdateStatus("test-job-123", StatusPending); err == nil {
		t.Error("Expected error for invalid transition, got nil")
	}

	// Valid transition: RUNNING -> SUCCEEDED
	if err := store.UpdateStatus("test-job-123", StatusSucceeded); err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Terminal state cannot transition
	if err := store.UpdateStatus("test-job-123", StatusRunning); err == nil {
		t.Error("Expected error for transition from terminal state, got nil")
	}
}

func TestStore_UpdateAssignment(t *testing.T) {
	store := setupTestStore(t)

	job := &Job{
		JobID:        "test-job-123",
		CreatedAt:    time.Now(),
		Status:       StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "input-key",
		OutputBucket: "output-bucket",
		OutputKey:    "output-key",
		AttemptID:    1,
	}

	// Create job
	if err := store.Create(job); err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Update assignment
	leaseDeadline := time.Now().Add(60 * time.Second)
	if err := store.UpdateAssignment("test-job-123", "agent-1", "lease-1", &leaseDeadline); err != nil {
		t.Fatalf("Failed to update assignment: %v", err)
	}

	retrieved, err := store.Get("test-job-123")
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if retrieved.AssignedAgentID != "agent-1" {
		t.Errorf("AssignedAgentID = %v, want agent-1", retrieved.AssignedAgentID)
	}
	if retrieved.LeaseID != "lease-1" {
		t.Errorf("LeaseID = %v, want lease-1", retrieved.LeaseID)
	}
	if retrieved.LeaseDeadline == nil {
		t.Error("LeaseDeadline is nil")
	}
}

func TestStore_List(t *testing.T) {
	store := setupTestStore(t)

	// Create multiple jobs
	jobs := []*Job{
		{
			JobID:        "job-1",
			CreatedAt:    time.Now(),
			Status:       StatusPending,
			InputBucket:  "bucket",
			InputKey:     "key1",
			OutputBucket: "bucket",
			OutputKey:    "out1",
			AttemptID:    1,
		},
		{
			JobID:        "job-2",
			CreatedAt:    time.Now().Add(time.Second),
			Status:       StatusRunning,
			InputBucket:  "bucket",
			InputKey:     "key2",
			OutputBucket: "bucket",
			OutputKey:    "out2",
			AttemptID:    1,
		},
		{
			JobID:        "job-3",
			CreatedAt:    time.Now().Add(2 * time.Second),
			Status:       StatusSucceeded,
			InputBucket:  "bucket",
			InputKey:     "key3",
			OutputBucket: "bucket",
			OutputKey:    "out3",
			AttemptID:    1,
		},
	}

	for _, j := range jobs {
		if err := store.Create(j); err != nil {
			t.Fatalf("Failed to create job %s: %v", j.JobID, err)
		}
	}

	// List all jobs
	allJobs, err := store.List(10, 0, nil)
	if err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}

	if len(allJobs) != 3 {
		t.Errorf("List() returned %d jobs, want 3", len(allJobs))
	}

	// List with status filter
	pendingJobs, err := store.List(10, 0, func() *Status { s := StatusPending; return &s }())
	if err != nil {
		t.Fatalf("Failed to list pending jobs: %v", err)
	}

	if len(pendingJobs) != 1 {
		t.Errorf("List(PENDING) returned %d jobs, want 1", len(pendingJobs))
	}
	if pendingJobs[0].JobID != "job-1" {
		t.Errorf("List(PENDING) returned job %s, want job-1", pendingJobs[0].JobID)
	}

	// List with limit and offset
	limitedJobs, err := store.List(2, 0, nil)
	if err != nil {
		t.Fatalf("Failed to list jobs with limit: %v", err)
	}

	if len(limitedJobs) != 2 {
		t.Errorf("List(limit=2) returned %d jobs, want 2", len(limitedJobs))
	}

	// Verify sorting: should be ordered by created_at DESC (newest first)
	// job-3 was created last (2 seconds after job-1), so should be first
	if len(limitedJobs) >= 2 {
		if limitedJobs[0].JobID != "job-3" {
			t.Errorf("First job should be job-3 (newest), got %s", limitedJobs[0].JobID)
		}
		if limitedJobs[1].JobID != "job-2" {
			t.Errorf("Second job should be job-2, got %s", limitedJobs[1].JobID)
		}
		// Verify descending order
		if limitedJobs[0].CreatedAt.Before(limitedJobs[1].CreatedAt) {
			t.Error("Jobs should be sorted by created_at DESC (newest first)")
		}
	}

	// Test offset
	offsetJobs, err := store.List(2, 1, nil)
	if err != nil {
		t.Fatalf("Failed to list jobs with offset: %v", err)
	}

	if len(offsetJobs) != 2 {
		t.Errorf("List(limit=2, offset=1) returned %d jobs, want 2", len(offsetJobs))
	}

	// With offset=1, should skip the first (newest) job
	if len(offsetJobs) >= 1 {
		if offsetJobs[0].JobID != "job-2" {
			t.Errorf("First job with offset=1 should be job-2, got %s", offsetJobs[0].JobID)
		}
	}
}

func TestStore_GetNotFound(t *testing.T) {
	store := setupTestStore(t)

	_, err := store.Get("non-existent-job")
	if !errors.Is(err, ErrJobNotFound) {
		t.Errorf("Get() error = %v, want ErrJobNotFound", err)
	}
}
