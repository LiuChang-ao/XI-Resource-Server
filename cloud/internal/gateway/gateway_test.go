package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xiresource/cloud/internal/job"
	"github.com/xiresource/cloud/internal/queue"
	"github.com/xiresource/cloud/internal/registry"
	control "github.com/xiresource/proto/control"
	"google.golang.org/protobuf/proto"
)

// mockRegistry implements Registry for testing
type mockRegistry struct {
	agents map[string]*registry.AgentInfo
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		agents: make(map[string]*registry.AgentInfo),
	}
}

func (m *mockRegistry) Register(agentID, hostname string, maxConcurrency int) {
	m.agents[agentID] = &registry.AgentInfo{
		AgentID:        agentID,
		Hostname:       hostname,
		MaxConcurrency: maxConcurrency,
		LastHeartbeat:  time.Now(),
		ConnectedAt:    time.Now(),
	}
}

func (m *mockRegistry) UpdateHeartbeat(agentID string, paused bool, runningJobs int) {
	if agent, exists := m.agents[agentID]; exists {
		agent.LastHeartbeat = time.Now()
		agent.Paused = paused
		agent.RunningJobs = runningJobs
	}
}

func (m *mockRegistry) Unregister(agentID string) {
	delete(m.agents, agentID)
}

func (m *mockRegistry) GetAgent(agentID string) (*registry.AgentInfo, bool) {
	agent, exists := m.agents[agentID]
	return agent, exists
}

// mockJobStore implements job.Store for testing
type mockJobStore struct {
	jobs map[string]*job.Job
}

func newMockJobStore() *mockJobStore {
	return &mockJobStore{
		jobs: make(map[string]*job.Job),
	}
}

func (m *mockJobStore) Create(j *job.Job) error {
	m.jobs[j.JobID] = j
	return nil
}

func (m *mockJobStore) Get(jobID string) (*job.Job, error) {
	j, exists := m.jobs[jobID]
	if !exists {
		return nil, job.ErrJobNotFound
	}
	return j, nil
}

func (m *mockJobStore) UpdateStatus(jobID string, newStatus job.Status) error {
	j, exists := m.jobs[jobID]
	if !exists {
		return job.ErrJobNotFound
	}
	if !j.Status.CanTransitionTo(newStatus) {
		return errors.New("invalid transition")
	}
	j.Status = newStatus
	return nil
}

func (m *mockJobStore) UpdateAssignment(jobID string, agentID string, leaseID string, leaseDeadline *time.Time) error {
	j, exists := m.jobs[jobID]
	if !exists {
		return job.ErrJobNotFound
	}
	j.AssignedAgentID = agentID
	j.LeaseID = leaseID
	j.LeaseDeadline = leaseDeadline
	return nil
}

func (m *mockJobStore) UpdateOutput(jobID string, outputKey, outputPrefix string) error {
	j, exists := m.jobs[jobID]
	if !exists {
		return job.ErrJobNotFound
	}
	j.OutputKey = outputKey
	j.OutputPrefix = outputPrefix
	return nil
}

func (m *mockJobStore) UpdateAttemptID(jobID string, attemptID int) error {
	j, exists := m.jobs[jobID]
	if !exists {
		return job.ErrJobNotFound
	}
	j.AttemptID = attemptID
	return nil
}

func (m *mockJobStore) UpdateStdoutStderr(jobID string, stdout, stderr string) error {
	j, exists := m.jobs[jobID]
	if !exists {
		return job.ErrJobNotFound
	}
	j.Stdout = stdout
	j.Stderr = stderr
	return nil
}

func (m *mockJobStore) List(limit int, offset int, status *job.Status) ([]*job.Job, error) {
	// Not needed for this test
	return nil, nil
}

func (m *mockJobStore) Close() error {
	return nil
}

// mockQueue implements queue.Queue for testing
type mockQueue struct {
	jobs []string
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		jobs: make([]string, 0),
	}
}

func (m *mockQueue) Enqueue(ctx context.Context, jobID string) error {
	m.jobs = append(m.jobs, jobID)
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (string, error) {
	if len(m.jobs) == 0 {
		return "", queue.ErrQueueEmpty
	}
	jobID := m.jobs[0]
	m.jobs = m.jobs[1:]
	return jobID, nil
}

func (m *mockQueue) Peek(ctx context.Context) (string, error) {
	if len(m.jobs) == 0 {
		return "", queue.ErrQueueEmpty
	}
	return m.jobs[0], nil
}

func (m *mockQueue) Size(ctx context.Context) (int64, error) {
	return int64(len(m.jobs)), nil
}

func (m *mockQueue) Remove(ctx context.Context, jobID string) error {
	for i, id := range m.jobs {
		if id == jobID {
			m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
			return nil
		}
	}
	return queue.ErrJobNotInQueue
}

// mockOSSProvider implements oss.Provider for testing
type mockOSSProvider struct{}

func newMockOSSProvider() *mockOSSProvider {
	return &mockOSSProvider{}
}

func (m *mockOSSProvider) GenerateDownloadURL(ctx context.Context, key string) (string, error) {
	return "https://example.com/download/" + key + "?presigned=yes", nil
}

func (m *mockOSSProvider) GenerateUploadURL(ctx context.Context, key string) (string, error) {
	return "https://example.com/upload/" + key + "?presigned=yes", nil
}

func (m *mockOSSProvider) GenerateUploadURLWithPrefix(ctx context.Context, prefix, filename string) (string, error) {
	key := prefix
	if len(key) > 0 && key[len(key)-1] != '/' {
		key += "/"
	}
	key += filename
	return m.GenerateUploadURL(ctx, key)
}

func TestGateway_HandleRequestJob(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 0)

	// Create a job and add it to the queue
	jobID := "job-123"
	testJob := &job.Job{
		JobID:        jobID,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-123/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	mockStore.Create(testJob)
	mockQueue.Enqueue(context.Background(), jobID)

	// Create agent connection (simplified for testing)
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create RequestJob message
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}

	// Process RequestJob
	gw.handleRequestJob(agentConn, envelope, envelope.GetRequestJob())

	// Check that job was assigned
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusAssigned {
		t.Errorf("Job status = %v, want %v", updatedJob.Status, job.StatusAssigned)
	}

	if updatedJob.AssignedAgentID != agentID {
		t.Errorf("AssignedAgentID = %v, want %v", updatedJob.AssignedAgentID, agentID)
	}

	if updatedJob.LeaseID == "" {
		t.Error("LeaseID should not be empty")
	}

	expectedPrefix := "jobs/" + jobID + "/1/"
	if updatedJob.OutputPrefix != expectedPrefix {
		t.Errorf("OutputPrefix = %v, want %v", updatedJob.OutputPrefix, expectedPrefix)
	}

	expectedKey := expectedPrefix + "output.bin"
	if updatedJob.OutputKey != expectedKey {
		t.Errorf("OutputKey = %v, want %v", updatedJob.OutputKey, expectedKey)
	}

	// Check that JobAssigned message was sent
	select {
	case msg := <-agentConn.SendChan:
		var assignedEnvelope control.Envelope
		if err := proto.Unmarshal(msg, &assignedEnvelope); err != nil {
			t.Fatalf("Failed to unmarshal JobAssigned: %v", err)
		}

		jobAssigned := assignedEnvelope.GetJobAssigned()
		if jobAssigned == nil {
			t.Fatal("Expected JobAssigned message")
		}

		if jobAssigned.JobId != jobID {
			t.Errorf("JobAssigned.JobId = %v, want %v", jobAssigned.JobId, jobID)
		}

		if jobAssigned.AttemptId != 1 {
			t.Errorf("JobAssigned.AttemptId = %v, want %v", jobAssigned.AttemptId, 1)
		}

		if jobAssigned.OutputPrefix != expectedPrefix {
			t.Errorf("JobAssigned.OutputPrefix = %v, want %v", jobAssigned.OutputPrefix, expectedPrefix)
		}

		if jobAssigned.OutputKey != expectedKey {
			t.Errorf("JobAssigned.OutputKey = %v, want %v", jobAssigned.OutputKey, expectedKey)
		}

		if jobAssigned.InputDownload == nil {
			t.Fatal("InputDownload should not be nil")
		}

		if jobAssigned.OutputUpload == nil {
			t.Fatal("OutputUpload should not be nil")
		}

		// Verify input_key is included in JobAssigned message
		if jobAssigned.InputKey != testJob.InputKey {
			t.Errorf("JobAssigned.InputKey = %v, want %v", jobAssigned.InputKey, testJob.InputKey)
		}

	case <-time.After(1 * time.Second):
		t.Fatal("No JobAssigned message received")
	}
}

func TestGateway_HandleRequestJob_EmptyQueue(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue() // Empty queue
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 0)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create RequestJob message
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}

	// Process RequestJob (should not send anything when queue is empty)
	gw.handleRequestJob(agentConn, envelope, envelope.GetRequestJob())

	// Check that no message was sent
	select {
	case <-agentConn.SendChan:
		t.Fatal("Should not send message when queue is empty")
	case <-time.After(100 * time.Millisecond):
		// Expected - no message should be sent
	}
}

func TestGateway_HandleRequestJob_PausedAgent(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent as paused
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, true, 0) // Paused

	// Create a job and add it to the queue
	jobID := "job-123"
	testJob := &job.Job{
		JobID:        jobID,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-123/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	mockStore.Create(testJob)
	mockQueue.Enqueue(context.Background(), jobID)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create RequestJob message
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}

	// Process RequestJob (should not assign job to paused agent)
	gw.handleRequestJob(agentConn, envelope, envelope.GetRequestJob())

	// Check that job was not assigned
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusPending {
		t.Errorf("Job status = %v, want %v (should not be assigned to paused agent)", updatedJob.Status, job.StatusPending)
	}

	// Check that no message was sent
	select {
	case <-agentConn.SendChan:
		t.Fatal("Should not send message to paused agent")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestGateway_HandleJobStatus_RUNNING(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent and assign a job
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)

	jobID := "job-123"
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusAssigned,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		AttemptID:       1,
		AssignedAgentID: agentID,
	}
	mockStore.Create(testJob)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create JobStatus RUNNING message
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: 1,
				Status:    control.JobStatusEnum_JOB_STATUS_RUNNING,
			},
		},
	}

	// Process JobStatus
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status was updated to RUNNING
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusRunning {
		t.Errorf("Job status = %v, want %v", updatedJob.Status, job.StatusRunning)
	}
}

func TestGateway_HandleJobStatus_SUCCEEDED(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent and assign a job
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 1) // Set RunningJobs=1 to test decrement

	jobID := "job-123"
	attemptID := 1
	outputKey := fmt.Sprintf("jobs/%s/%d/output.bin", jobID, attemptID)
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusRunning,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		AttemptID:       attemptID,
		AssignedAgentID: agentID,
		OutputKey:       outputKey, // Set OutputKey for presigned mode validation
		OutputPrefix:    fmt.Sprintf("jobs/%s/%d/", jobID, attemptID),
	}
	mockStore.Create(testJob)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create JobStatus SUCCEEDED message with valid output_key (must match store.OutputKey)
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: int32(attemptID),
				Status:    control.JobStatusEnum_JOB_STATUS_SUCCEEDED,
				OutputKey: outputKey,
			},
		},
	}

	// Process JobStatus
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status was updated to SUCCEEDED
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusSucceeded {
		t.Errorf("Job status = %v, want %v", updatedJob.Status, job.StatusSucceeded)
	}

	if updatedJob.OutputKey != outputKey {
		t.Errorf("OutputKey = %v, want %v", updatedJob.OutputKey, outputKey)
	}

	expectedPrefix := fmt.Sprintf("jobs/%s/%d/", jobID, attemptID)
	if updatedJob.OutputPrefix != expectedPrefix {
		t.Errorf("OutputPrefix = %v, want %v", updatedJob.OutputPrefix, expectedPrefix)
	}

	// Verify RunningJobs was decremented
	agentInfo, exists := mockReg.GetAgent(agentID)
	if !exists {
		t.Fatal("Agent should still exist")
	}
	if agentInfo.RunningJobs != 0 {
		t.Errorf("RunningJobs = %v, want 0 (should be decremented on terminal state)", agentInfo.RunningJobs)
	}
}

func TestGateway_HandleJobStatus_SUCCEEDED_MissingOutputKey(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent and assign a job
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 1) // Set RunningJobs=1 to test decrement

	jobID := "job-123"
	expectedOutputKey := "jobs/job-123/1/output.bin"
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusRunning,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		OutputKey:       expectedOutputKey, // Store has OutputKey (from job assignment)
		AttemptID:       1,
		AssignedAgentID: agentID,
	}
	mockStore.Create(testJob)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create JobStatus SUCCEEDED message without output_key (command only produces stdout)
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: 1,
				Status:    control.JobStatusEnum_JOB_STATUS_SUCCEEDED,
				OutputKey: "", // No output file, only stdout (this is allowed)
				Stdout:    "Command output",
			},
		},
	}

	// Process JobStatus (should accept and mark as SUCCEEDED - output_key is optional)
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status was updated to SUCCEEDED (output_key is optional)
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusSucceeded {
		t.Errorf("Job status = %v, want %v (should be SUCCEEDED when output_key is missing but command produces stdout)", updatedJob.Status, job.StatusSucceeded)
	}

	// Verify stdout was saved
	if updatedJob.Stdout != "Command output" {
		t.Errorf("Stdout = %v, want %v", updatedJob.Stdout, "Command output")
	}

	// Verify RunningJobs was decremented
	agentInfo, exists := mockReg.GetAgent(agentID)
	if !exists {
		t.Fatal("Agent should still exist")
	}
	if agentInfo.RunningJobs != 0 {
		t.Errorf("RunningJobs = %v, want 0 (should be decremented on terminal state)", agentInfo.RunningJobs)
	}
}

func TestGateway_HandleJobStatus_SUCCEEDED_InvalidOutputKey(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent and assign a job
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 1) // Set RunningJobs=1 to test decrement

	jobID := "job-123"
	attemptID := 1
	expectedOutputKey := fmt.Sprintf("jobs/%s/%d/output.bin", jobID, attemptID)
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusRunning,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		OutputKey:       expectedOutputKey, // Store has expected output key
		AttemptID:       attemptID,
		AssignedAgentID: agentID,
	}
	mockStore.Create(testJob)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create JobStatus SUCCEEDED message with invalid output_key (doesn't match store.OutputKey)
	invalidOutputKey := "invalid/path/output.bin"
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: int32(attemptID),
				Status:    control.JobStatusEnum_JOB_STATUS_SUCCEEDED,
				OutputKey: invalidOutputKey, // Doesn't match expectedOutputKey
			},
		},
	}

	// Process JobStatus (should reject and mark as FAILED because output_key doesn't match)
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status was updated to FAILED (not SUCCEEDED)
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusFailed {
		t.Errorf("Job status = %v, want %v (should be FAILED when output_key doesn't match store.OutputKey)", updatedJob.Status, job.StatusFailed)
	}

	// Verify RunningJobs was decremented
	agentInfo, exists := mockReg.GetAgent(agentID)
	if !exists {
		t.Fatal("Agent should still exist")
	}
	if agentInfo.RunningJobs != 0 {
		t.Errorf("RunningJobs = %v, want 0 (should be decremented on terminal state)", agentInfo.RunningJobs)
	}
}

func TestGateway_HandleJobStatus_FAILED(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent and assign a job
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)

	jobID := "job-123"
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusRunning,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		AttemptID:       1,
		AssignedAgentID: agentID,
	}
	mockStore.Create(testJob)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Create JobStatus FAILED message
	failureMessage := "Task execution failed"
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: 1,
				Status:    control.JobStatusEnum_JOB_STATUS_FAILED,
				Message:   failureMessage,
			},
		},
	}

	// Process JobStatus
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status was updated to FAILED
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusFailed {
		t.Errorf("Job status = %v, want %v", updatedJob.Status, job.StatusFailed)
	}
}

func TestGateway_HandleJobStatus_TerminalStateProtection(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register an agent and assign a job
	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)

	jobID := "job-123"
	attemptID := 1
	outputKey := fmt.Sprintf("jobs/%s/%d/output.bin", jobID, attemptID)
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusSucceeded, // Already in terminal state
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		AttemptID:       attemptID,
		AssignedAgentID: agentID,
		OutputKey:       outputKey,
		OutputPrefix:    fmt.Sprintf("jobs/%s/%d/", jobID, attemptID),
	}
	mockStore.Create(testJob)

	// Create agent connection
	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Try to update to RUNNING (should be ignored)
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: int32(attemptID),
				Status:    control.JobStatusEnum_JOB_STATUS_RUNNING,
			},
		},
	}

	// Process JobStatus (should ignore the update)
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status remains SUCCEEDED (not changed to RUNNING)
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusSucceeded {
		t.Errorf("Job status = %v, want %v (terminal state should not be overwritten)", updatedJob.Status, job.StatusSucceeded)
	}
}

func TestGateway_HandleJobStatus_WrongAgent(t *testing.T) {
	// Setup
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	// Register two agents
	agentID1 := "agent-123"
	agentID2 := "agent-456"
	mockReg.Register(agentID1, "test-host-1", 1)
	mockReg.Register(agentID2, "test-host-2", 1)

	// Job assigned to agent1
	jobID := "job-123"
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusRunning,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		AttemptID:       1,
		AssignedAgentID: agentID1, // Assigned to agent1
	}
	mockStore.Create(testJob)

	// Create agent connection for agent2
	agentConn := &AgentConnection{
		AgentID:   agentID2,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// agent2 tries to report status for job assigned to agent1
	envelope := &control.Envelope{
		AgentId:   agentID2,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: 1,
				Status:    control.JobStatusEnum_JOB_STATUS_SUCCEEDED,
				OutputKey: fmt.Sprintf("jobs/%s/1/output.bin", jobID),
			},
		},
	}

	// Process JobStatus (should reject)
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Check that job status was NOT updated (still RUNNING)
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusRunning {
		t.Errorf("Job status = %v, want %v (should not be updated by wrong agent)", updatedJob.Status, job.StatusRunning)
	}
}

// Fix 1: Test attempt_id consistency - job with AttemptID=0 should be updated to 1
func TestGateway_HandleRequestJob_AttemptIDConsistency(t *testing.T) {
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 0)

	// Create job with AttemptID=0 (default/initial value)
	jobID := "job-123"
	testJob := &job.Job{
		JobID:        jobID,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-123/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    0, // Initial value that needs fixing
	}
	mockStore.Create(testJob)
	mockQueue.Enqueue(context.Background(), jobID)

	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}

	// Process RequestJob
	gw.handleRequestJob(agentConn, envelope, envelope.GetRequestJob())

	// Verify job was assigned and AttemptID was updated to 1
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.AttemptID != 1 {
		t.Errorf("AttemptID = %v, want 1 (should be updated from 0)", updatedJob.AttemptID)
	}

	if updatedJob.Status != job.StatusAssigned {
		t.Errorf("Job status = %v, want %v", updatedJob.Status, job.StatusAssigned)
	}

	// Verify JobAssigned message has attempt_id=1
	select {
	case msg := <-agentConn.SendChan:
		var assignedEnvelope control.Envelope
		if err := proto.Unmarshal(msg, &assignedEnvelope); err != nil {
			t.Fatalf("Failed to unmarshal JobAssigned: %v", err)
		}
		jobAssigned := assignedEnvelope.GetJobAssigned()
		if jobAssigned == nil {
			t.Fatal("Expected JobAssigned message")
		}
		if jobAssigned.AttemptId != 1 {
			t.Errorf("JobAssigned.AttemptId = %v, want 1", jobAssigned.AttemptId)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("No JobAssigned message received")
	}

	// Now test that agent can report status with attempt_id=1 and it's accepted
	statusEnvelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: 1,
				Status:    control.JobStatusEnum_JOB_STATUS_RUNNING,
			},
		},
	}
	gw.handleJobStatus(agentConn, statusEnvelope, statusEnvelope.GetJobStatus())

	updatedJob2, _ := mockStore.Get(jobID)
	if updatedJob2.Status != job.StatusRunning {
		t.Errorf("After status report, job status = %v, want %v (should accept attempt_id=1)", updatedJob2.Status, job.StatusRunning)
	}
}

// Fix 2: Test Dequeue skips non-PENDING jobs and finds valid PENDING job
func TestGateway_HandleRequestJob_DequeueSkipsNonPending(t *testing.T) {
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 0)

	// Create two jobs: one ASSIGNED, one PENDING
	jobA := "job-A"
	jobB := "job-B"
	testJobA := &job.Job{
		JobID:        jobA,
		CreatedAt:    time.Now(),
		Status:       job.StatusAssigned, // Non-PENDING
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-A/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	testJobB := &job.Job{
		JobID:        jobB,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending, // Valid PENDING
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-B/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	mockStore.Create(testJobA)
	mockStore.Create(testJobB)

	// Enqueue in order: A (non-PENDING) first, then B (PENDING)
	mockQueue.Enqueue(context.Background(), jobA)
	mockQueue.Enqueue(context.Background(), jobB)

	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}

	// Process RequestJob - should skip jobA and assign jobB
	gw.handleRequestJob(agentConn, envelope, envelope.GetRequestJob())

	// Verify jobB was assigned (not jobA)
	jobBUpdated, err := mockStore.Get(jobB)
	if err != nil {
		t.Fatalf("Failed to get jobB: %v", err)
	}
	if jobBUpdated.Status != job.StatusAssigned {
		t.Errorf("jobB status = %v, want %v", jobBUpdated.Status, job.StatusAssigned)
	}

	// Verify jobA was NOT assigned
	jobAUpdated, err := mockStore.Get(jobA)
	if err != nil {
		t.Fatalf("Failed to get jobA: %v", err)
	}
	if jobAUpdated.Status != job.StatusAssigned {
		t.Logf("jobA status = %v (correctly not assigned)", jobAUpdated.Status)
	}

	// Verify JobAssigned message is for jobB
	select {
	case msg := <-agentConn.SendChan:
		var assignedEnvelope control.Envelope
		if err := proto.Unmarshal(msg, &assignedEnvelope); err != nil {
			t.Fatalf("Failed to unmarshal JobAssigned: %v", err)
		}
		jobAssigned := assignedEnvelope.GetJobAssigned()
		if jobAssigned == nil {
			t.Fatal("Expected JobAssigned message")
		}
		if jobAssigned.JobId != jobB {
			t.Errorf("JobAssigned.JobId = %v, want %v", jobAssigned.JobId, jobB)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("No JobAssigned message received")
	}
}

// Fix 3: Test failure compensation - re-enqueue on presigned URL generation failure
func TestGateway_HandleRequestJob_FailureCompensation(t *testing.T) {
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	// Create a failing OSS provider
	failingOSS := &failingOSSProvider{}
	gw := New(mockReg, mockStore, mockQueue, failingOSS, true)

	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 0)

	jobID := "job-123"
	testJob := &job.Job{
		JobID:        jobID,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-123/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	mockStore.Create(testJob)
	mockQueue.Enqueue(context.Background(), jobID)

	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}

	// Process RequestJob - should fail at presigned URL generation and re-enqueue
	gw.handleRequestJob(agentConn, envelope, envelope.GetRequestJob())

	// Verify job is still PENDING (not assigned)
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}
	if updatedJob.Status != job.StatusPending {
		t.Errorf("Job status = %v, want %v (should remain PENDING after failure)", updatedJob.Status, job.StatusPending)
	}

	// Verify job was re-enqueued (queue should have the job back)
	queueSize, _ := mockQueue.Size(context.Background())
	if queueSize != 1 {
		t.Errorf("Queue size = %v, want 1 (job should be re-enqueued)", queueSize)
	}

	// Verify no JobAssigned message was sent
	select {
	case <-agentConn.SendChan:
		t.Fatal("Should not send JobAssigned message when assignment fails")
	case <-time.After(100 * time.Millisecond):
		// Expected - no message should be sent
	}
}

// failingOSSProvider implements oss.Provider but always fails
type failingOSSProvider struct{}

func (f *failingOSSProvider) GenerateDownloadURL(ctx context.Context, key string) (string, error) {
	return "", fmt.Errorf("mock OSS provider failure")
}

func (f *failingOSSProvider) GenerateUploadURL(ctx context.Context, key string) (string, error) {
	return "", fmt.Errorf("mock OSS provider failure")
}

func (f *failingOSSProvider) GenerateUploadURLWithPrefix(ctx context.Context, prefix, filename string) (string, error) {
	return "", fmt.Errorf("mock OSS provider failure")
}

// Fix 4: Test SUCCEEDED with output_key mismatch (should reject and mark FAILED)
func TestGateway_HandleJobStatus_SUCCEEDED_OutputKeyMismatch(t *testing.T) {
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	agentID := "agent-123"
	mockReg.Register(agentID, "test-host", 1)
	mockReg.UpdateHeartbeat(agentID, false, 1) // Set RunningJobs=1 to test decrement

	jobID := "job-123"
	attemptID := 1
	expectedOutputKey := fmt.Sprintf("jobs/%s/%d/output.bin", jobID, attemptID)
	// Store has expectedOutputKey
	testJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusRunning,
		InputBucket:     "input-bucket",
		InputKey:        "inputs/job-123/input.bin",
		OutputBucket:    "output-bucket",
		AttemptID:       attemptID,
		AssignedAgentID: agentID,
		OutputKey:       expectedOutputKey, // Store has this key
		OutputPrefix:    fmt.Sprintf("jobs/%s/%d/", jobID, attemptID),
	}
	mockStore.Create(testJob)

	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// Agent reports SUCCEEDED with different output_key (same prefix, different filename)
	wrongOutputKey := fmt.Sprintf("jobs/%s/%d/wrong-output.bin", jobID, attemptID)
	envelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     jobID,
				AttemptId: int32(attemptID),
				Status:    control.JobStatusEnum_JOB_STATUS_SUCCEEDED,
				OutputKey: wrongOutputKey, // Different key
			},
		},
	}

	// Process JobStatus - should reject and mark FAILED
	gw.handleJobStatus(agentConn, envelope, envelope.GetJobStatus())

	// Verify job status is FAILED (not SUCCEEDED)
	updatedJob, err := mockStore.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	if updatedJob.Status != job.StatusFailed {
		t.Errorf("Job status = %v, want %v (should be FAILED when output_key mismatches)", updatedJob.Status, job.StatusFailed)
	}

	// Verify store.OutputKey was NOT polluted (should still be expectedOutputKey)
	if updatedJob.OutputKey != expectedOutputKey {
		t.Errorf("OutputKey = %v, want %v (should not be polluted by wrong key)", updatedJob.OutputKey, expectedOutputKey)
	}

	// Verify RunningJobs was decremented
	agentInfo, _ := mockReg.GetAgent(agentID)
	if agentInfo != nil && agentInfo.RunningJobs != 0 {
		t.Errorf("RunningJobs = %v, want 0 (should be decremented on terminal state)", agentInfo.RunningJobs)
	}
}

// Fix 5: Test RunningJobs increment on assignment and decrement on terminal state
func TestGateway_RunningJobsCapacitySync(t *testing.T) {
	mockReg := newMockRegistry()
	mockStore := newMockJobStore()
	mockQueue := newMockQueue()
	mockOSS := newMockOSSProvider()
	gw := New(mockReg, mockStore, mockQueue, mockOSS, true)

	agentID := "agent-123"
	maxConcurrency := 1
	mockReg.Register(agentID, "test-host", maxConcurrency)
	mockReg.UpdateHeartbeat(agentID, false, 0) // Start with 0 running jobs

	// Create two jobs
	job1 := "job-1"
	job2 := "job-2"
	testJob1 := &job.Job{
		JobID:        job1,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-1/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	testJob2 := &job.Job{
		JobID:        job2,
		CreatedAt:    time.Now(),
		Status:       job.StatusPending,
		InputBucket:  "input-bucket",
		InputKey:     "inputs/job-2/input.bin",
		OutputBucket: "output-bucket",
		AttemptID:    1,
	}
	mockStore.Create(testJob1)
	mockStore.Create(testJob2)
	mockQueue.Enqueue(context.Background(), job1)
	mockQueue.Enqueue(context.Background(), job2)

	agentConn := &AgentConnection{
		AgentID:   agentID,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	// First RequestJob - should assign job1 and increment RunningJobs
	envelope1 := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}
	gw.handleRequestJob(agentConn, envelope1, envelope1.GetRequestJob())

	// Verify RunningJobs was incremented
	agentInfo, _ := mockReg.GetAgent(agentID)
	if agentInfo == nil {
		t.Fatal("Agent should exist")
	}
	if agentInfo.RunningJobs != 1 {
		t.Errorf("After first assignment, RunningJobs = %v, want 1", agentInfo.RunningJobs)
	}

	// Verify job1 was assigned
	job1Updated, _ := mockStore.Get(job1)
	if job1Updated.Status != job.StatusAssigned {
		t.Errorf("job1 status = %v, want %v", job1Updated.Status, job.StatusAssigned)
	}

	// Second RequestJob - should NOT assign job2 (no capacity)
	envelope2 := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}
	gw.handleRequestJob(agentConn, envelope2, envelope2.GetRequestJob())

	// Verify job2 was NOT assigned
	job2Updated, _ := mockStore.Get(job2)
	if job2Updated.Status != job.StatusPending {
		t.Errorf("job2 status = %v, want %v (should not be assigned when no capacity)", job2Updated.Status, job.StatusPending)
	}

	// Verify RunningJobs is still 1
	agentInfo, _ = mockReg.GetAgent(agentID)
	if agentInfo.RunningJobs != 1 {
		t.Errorf("After second request (no capacity), RunningJobs = %v, want 1", agentInfo.RunningJobs)
	}

	// Now report job1 as SUCCEEDED - should decrement RunningJobs
	statusEnvelope := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: &control.JobStatus{
				JobId:     job1,
				AttemptId: 1,
				Status:    control.JobStatusEnum_JOB_STATUS_SUCCEEDED,
				OutputKey: fmt.Sprintf("jobs/%s/1/output.bin", job1),
			},
		},
	}
	// Update job1 to RUNNING first (to match expected state)
	mockStore.UpdateStatus(job1, job.StatusRunning)
	gw.handleJobStatus(agentConn, statusEnvelope, statusEnvelope.GetJobStatus())

	// Verify RunningJobs was decremented
	agentInfo, _ = mockReg.GetAgent(agentID)
	if agentInfo.RunningJobs != 0 {
		t.Errorf("After job1 SUCCEEDED, RunningJobs = %v, want 0", agentInfo.RunningJobs)
	}

	// Now third RequestJob - should assign job2 (capacity available)
	envelope3 := &control.Envelope{
		AgentId:   agentID,
		RequestId: uuid.New().String(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: agentID,
			},
		},
	}
	gw.handleRequestJob(agentConn, envelope3, envelope3.GetRequestJob())

	// Verify job2 was assigned
	job2Updated2, _ := mockStore.Get(job2)
	if job2Updated2.Status != job.StatusAssigned {
		t.Errorf("After capacity freed, job2 status = %v, want %v", job2Updated2.Status, job.StatusAssigned)
	}

	// Verify RunningJobs was incremented again
	agentInfo, _ = mockReg.GetAgent(agentID)
	if agentInfo.RunningJobs != 1 {
		t.Errorf("After second assignment, RunningJobs = %v, want 1", agentInfo.RunningJobs)
	}
}
