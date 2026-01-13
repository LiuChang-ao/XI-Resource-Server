package gateway

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"
	"github.com/xiresource/cloud/internal/job"
	"github.com/xiresource/cloud/internal/oss"
	"github.com/xiresource/cloud/internal/queue"
	"github.com/xiresource/cloud/internal/registry"
	control "github.com/xiresource/proto/control"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in dev mode
	},
}

// AgentConnection represents a connected agent
type AgentConnection struct {
	AgentID   string
	Conn      *websocket.Conn
	SendChan  chan []byte
	CloseChan chan struct{}
}

// Gateway manages WebSocket connections from agents
type Gateway struct {
	registry    Registry
	jobStore    job.Store
	jobQueue    queue.Queue
	ossProvider oss.Provider
	connections map[string]*AgentConnection
	mu          sync.RWMutex
	devMode     bool
	agentTokens map[string]string // agent_id -> token_hash (MVP: plain for dev)
}

// Registry interface for agent tracking
type Registry interface {
	Register(agentID, hostname string, maxConcurrency int)
	UpdateHeartbeat(agentID string, paused bool, runningJobs int)
	Unregister(agentID string)
	GetAgent(agentID string) (*registry.AgentInfo, bool) // Returns agent info if registered and online
}

// New creates a new gateway
func New(registry Registry, jobStore job.Store, jobQueue queue.Queue, ossProvider oss.Provider, devMode bool) *Gateway {
	return &Gateway{
		registry:    registry,
		jobStore:    jobStore,
		jobQueue:    jobQueue,
		ossProvider: ossProvider,
		connections: make(map[string]*AgentConnection),
		devMode:     devMode,
		agentTokens: make(map[string]string), // MVP: accept any token in dev mode
	}
}

// HandleWebSocket handles incoming WebSocket connections
func (g *Gateway) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	agentConn := &AgentConnection{
		Conn:      conn,
		SendChan:  make(chan []byte, 256),
		CloseChan: make(chan struct{}),
	}

	go g.handleConnection(agentConn)
}

func (g *Gateway) handleConnection(agentConn *AgentConnection) {
	defer func() {
		if agentConn.AgentID != "" {
			g.mu.Lock()
			delete(g.connections, agentConn.AgentID)
			g.mu.Unlock()
			g.registry.Unregister(agentConn.AgentID)
			log.Printf("Agent %s disconnected", agentConn.AgentID)
		}
		agentConn.Conn.Close()
	}()

	// Start send goroutine
	go func() {
		for {
			select {
			case msg := <-agentConn.SendChan:
				if err := agentConn.Conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
					log.Printf("Write error: %v", err)
					return
				}
			case <-agentConn.CloseChan:
				return
			}
		}
	}()

	// Read loop
	for {
		messageType, data, err := agentConn.Conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		if messageType != websocket.BinaryMessage {
			log.Printf("Unexpected message type: %d", messageType)
			continue
		}

		var envelope control.Envelope
		if err := proto.Unmarshal(data, &envelope); err != nil {
			log.Printf("Failed to unmarshal envelope: %v", err)
			continue
		}

		g.handleMessage(agentConn, &envelope)
	}
}

func (g *Gateway) handleMessage(agentConn *AgentConnection, envelope *control.Envelope) {
	switch payload := envelope.Payload.(type) {
	case *control.Envelope_Register:
		g.handleRegister(agentConn, envelope, payload.Register)
	case *control.Envelope_Heartbeat:
		g.handleHeartbeat(agentConn, envelope, payload.Heartbeat)
	case *control.Envelope_RequestJob:
		g.handleRequestJob(agentConn, envelope, payload.RequestJob)
	case *control.Envelope_JobStatus:
		g.handleJobStatus(agentConn, envelope, payload.JobStatus)
	default:
		log.Printf("Unknown message type from agent %s", envelope.AgentId)
	}
}

func (g *Gateway) handleRegister(agentConn *AgentConnection, envelope *control.Envelope, reg *control.Register) {
	agentID := reg.AgentId
	if agentID == "" {
		log.Printf("Register message missing agent_id")
		return
	}

	// MVP: In dev mode, accept any token. In production, validate token hash.
	if !g.devMode {
		// TODO: Validate token hash
		log.Printf("Token validation not implemented yet")
	}

	// Register agent
	g.mu.Lock()
	agentConn.AgentID = agentID
	g.connections[agentID] = agentConn
	g.mu.Unlock()

	g.registry.Register(agentID, reg.Hostname, int(reg.MaxConcurrency))
	log.Printf("Agent %s registered (hostname: %s, max_concurrency: %d)", agentID, reg.Hostname, reg.MaxConcurrency)

	// Send RegisterAck
	ack := &control.Envelope{
		RequestId: envelope.RequestId,
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RegisterAck{
			RegisterAck: &control.RegisterAck{
				Success:              true,
				Message:              "Registered successfully",
				HeartbeatIntervalSec: 20,
			},
		},
	}

	ackData, err := proto.Marshal(ack)
	if err != nil {
		log.Printf("Failed to marshal RegisterAck: %v", err)
		return
	}

	agentConn.SendChan <- ackData
}

func (g *Gateway) handleHeartbeat(agentConn *AgentConnection, envelope *control.Envelope, hb *control.Heartbeat) {
	if agentConn.AgentID == "" {
		log.Printf("Heartbeat from unregistered agent")
		return
	}

	if hb.AgentId != agentConn.AgentID {
		log.Printf("Heartbeat agent_id mismatch: expected %s, got %s", agentConn.AgentID, hb.AgentId)
		return
	}

	g.registry.UpdateHeartbeat(hb.AgentId, hb.Paused, int(hb.RunningJobs))

	// Send HeartbeatAck
	ack := &control.Envelope{
		RequestId: envelope.RequestId,
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_HeartbeatAck{
			HeartbeatAck: &control.HeartbeatAck{
				Success: true,
			},
		},
	}

	ackData, err := proto.Marshal(ack)
	if err != nil {
		log.Printf("Failed to marshal HeartbeatAck: %v", err)
		return
	}

	agentConn.SendChan <- ackData
}

func (g *Gateway) handleRequestJob(agentConn *AgentConnection, envelope *control.Envelope, req *control.RequestJob) {
	// Validate agent_id consistency
	if req.AgentId != "" && req.AgentId != envelope.AgentId {
		log.Printf("RequestJob agent_id mismatch: envelope=%s, payload=%s", envelope.AgentId, req.AgentId)
		return
	}

	agentID := envelope.AgentId
	if agentID == "" {
		log.Printf("RequestJob missing agent_id in envelope")
		return
	}

	// Check if agent is registered and online
	agentInfo, exists := g.registry.GetAgent(agentID)
	if !exists {
		log.Printf("RequestJob from unregistered agent: %s", agentID)
		return
	}

	// Check if agent is paused or has no capacity
	if agentInfo.Paused {
		log.Printf("Agent %s is paused, skipping job assignment", agentID)
		return
	}

	if agentInfo.RunningJobs >= agentInfo.MaxConcurrency {
		log.Printf("Agent %s has no capacity (running=%d, max=%d), skipping job assignment",
			agentID, agentInfo.RunningJobs, agentInfo.MaxConcurrency)
		return
	}

	// Check if queue is available
	if g.jobQueue == nil {
		log.Printf("Job queue not available, skipping job assignment for agent %s", agentID)
		return
	}

	// Fix 2: Dequeue with retry loop (max 5 attempts) to skip non-PENDING jobs
	ctx := context.Background()
	const maxDequeueAttempts = 5
	var jobID string
	var j *job.Job
	var err error

	for attempt := 0; attempt < maxDequeueAttempts; attempt++ {
		jobID, err = g.jobQueue.Dequeue(ctx)
		if err == queue.ErrQueueEmpty {
			log.Printf("No job available for agent %s", agentID)
			return
		}
		if err != nil {
			log.Printf("Failed to dequeue job for agent %s: %v", agentID, err)
			return
		}

		// Get job from store
		j, err = g.jobStore.Get(jobID)
		if err != nil {
			log.Printf("Failed to get job %s: %v, trying next job", jobID, err)
			// Job may have been deleted, continue to next
			continue
		}

		// Validate job state is PENDING
		if j.Status != job.StatusPending {
			log.Printf("Job %s is not PENDING (status=%s), skipping and trying next job", jobID, j.Status)
			// Continue to try next job (this job may be in queue but already assigned)
			continue
		}

		// Found a valid PENDING job, break out of loop
		break
	}

	// If we exhausted attempts without finding a valid job, return
	if j == nil || j.Status != job.StatusPending {
		log.Printf("No valid PENDING job found after %d attempts for agent %s", maxDequeueAttempts, agentID)
		return
	}

	// Fix 1: Ensure attempt_id is 1 in store before assignment
	attemptID := 1
	if j.AttemptID < 1 {
		// Update store to ensure attempt_id=1
		if err := g.jobStore.UpdateAttemptID(jobID, 1); err != nil {
			log.Printf("Failed to update attempt_id for job %s: %v, re-enqueuing", jobID, err)
			// Re-enqueue job for retry
			_ = g.jobQueue.Enqueue(ctx, jobID)
			return
		}
		log.Printf("Updated attempt_id to 1 for job %s (was %d)", jobID, j.AttemptID)
		j.AttemptID = 1
	} else if j.AttemptID > 1 {
		attemptID = j.AttemptID
	}

	// Generate output prefix: jobs/{job_id}/{attempt_id}/
	outputPrefix := fmt.Sprintf("jobs/%s/%d/", jobID, attemptID)

	// Generate output key with extension (using presigned PUT)
	outputExtension := j.OutputExtension
	if outputExtension == "" {
		outputExtension = "bin" // Default extension
	}
	// Remove leading dot if present
	if strings.HasPrefix(outputExtension, ".") {
		outputExtension = outputExtension[1:]
	}
	outputKey := fmt.Sprintf("%soutput.%s", outputPrefix, outputExtension)

	// Fix 3: Generate presigned URLs with failure compensation
	inputDownloadURL, err := g.ossProvider.GenerateDownloadURL(ctx, j.InputKey)
	if err != nil {
		log.Printf("Failed to generate input download URL for job %s: %v, re-enqueuing", jobID, err)
		// Re-enqueue job for retry
		_ = g.jobQueue.Enqueue(ctx, jobID)
		return
	}

	outputUploadURL, err := g.ossProvider.GenerateUploadURL(ctx, outputKey)
	if err != nil {
		log.Printf("Failed to generate output upload URL for job %s: %v, re-enqueuing", jobID, err)
		// Re-enqueue job for retry
		_ = g.jobQueue.Enqueue(ctx, jobID)
		return
	}

	// Generate lease_id (M1: simple UUID, lease renewal not implemented yet)
	leaseID := uuid.New().String()
	leaseTTLSec := int32(60) // Default 60 seconds

	// Fix 3: Update job assignment with failure compensation
	err = g.jobStore.UpdateAssignment(jobID, agentID, leaseID, nil)
	if err != nil {
		log.Printf("Failed to update job assignment for job %s: %v, re-enqueuing", jobID, err)
		// Re-enqueue job for retry
		_ = g.jobQueue.Enqueue(ctx, jobID)
		return
	}

	// Fix 3: Update job status to ASSIGNED with failure compensation
	err = g.jobStore.UpdateStatus(jobID, job.StatusAssigned)
	if err != nil {
		log.Printf("Failed to update job status to ASSIGNED for job %s: %v, reverting assignment and re-enqueuing", jobID, err)
		// Revert assignment and re-enqueue
		_ = g.jobStore.UpdateAssignment(jobID, "", "", nil)
		_ = g.jobQueue.Enqueue(ctx, jobID)
		return
	}

	// Fix 3: Update output key/prefix with failure compensation
	err = g.jobStore.UpdateOutput(jobID, outputKey, outputPrefix)
	if err != nil {
		log.Printf("Failed to update output key/prefix for job %s: %v, reverting to PENDING and re-enqueuing", jobID, err)
		// Revert status and assignment, then re-enqueue
		_ = g.jobStore.UpdateStatus(jobID, job.StatusPending)
		_ = g.jobStore.UpdateAssignment(jobID, "", "", nil)
		_ = g.jobQueue.Enqueue(ctx, jobID)
		return
	}

	// Create JobAssigned message
	jobAssigned := &control.Envelope{
		RequestId: envelope.RequestId,
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobAssigned{
			JobAssigned: &control.JobAssigned{
				JobId:         jobID,
				AttemptId:     int32(attemptID),
				LeaseId:       leaseID,
				LeaseTtlSec:   leaseTTLSec,
				InputDownload: &control.OSSAccess{Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputDownloadURL}},
				InputKey:      j.InputKey,
				OutputUpload:  &control.OSSAccess{Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputUploadURL}},
				OutputPrefix:  outputPrefix,
				OutputKey:     outputKey,
				Command:       j.Command,
			},
		},
	}

	jobAssignedData, err := proto.Marshal(jobAssigned)
	if err != nil {
		log.Printf("Failed to marshal JobAssigned for job %s: %v, reverting to PENDING and re-enqueuing", jobID, err)
		// Revert status and assignment, then re-enqueue
		_ = g.jobStore.UpdateStatus(jobID, job.StatusPending)
		_ = g.jobStore.UpdateAssignment(jobID, "", "", nil)
		_ = g.jobQueue.Enqueue(ctx, jobID)
		return
	}

	// Fix 5: Increment RunningJobs after successful assignment
	g.registry.UpdateHeartbeat(agentID, agentInfo.Paused, agentInfo.RunningJobs+1)

	log.Printf("Assigned job %s (attempt %d) to agent %s", jobID, attemptID, agentID)
	agentConn.SendChan <- jobAssignedData
}

func (g *Gateway) handleJobStatus(agentConn *AgentConnection, envelope *control.Envelope, status *control.JobStatus) {
	agentID := envelope.AgentId
	if agentID == "" {
		log.Printf("JobStatus missing agent_id in envelope")
		return
	}

	if agentConn.AgentID != agentID {
		log.Printf("JobStatus agent_id mismatch: connection=%s, envelope=%s", agentConn.AgentID, agentID)
		return
	}

	jobID := status.JobId
	if jobID == "" {
		log.Printf("JobStatus missing job_id from agent %s", agentID)
		return
	}

	// Get job from store
	j, err := g.jobStore.Get(jobID)
	if err == job.ErrJobNotFound {
		log.Printf("JobStatus: job %s not found from agent %s", jobID, agentID)
		return
	}
	if err != nil {
		log.Printf("Failed to get job %s: %v", jobID, err)
		return
	}

	// Validate that job belongs to this agent
	if j.AssignedAgentID != agentID {
		log.Printf("JobStatus: job %s does not belong to agent %s (assigned to %s)", jobID, agentID, j.AssignedAgentID)
		return
	}

	// Validate attempt_id (M1: must be 1, but we also check against stored value)
	attemptID := int(status.AttemptId)
	if attemptID != j.AttemptID {
		log.Printf("JobStatus: attempt_id mismatch for job %s: reported=%d, expected=%d", jobID, attemptID, j.AttemptID)
		return
	}

	// Convert protobuf status to job.Status
	newStatus := jobStatusFromProto(status.Status)
	if !newStatus.IsValid() {
		log.Printf("JobStatus: invalid status %v for job %s from agent %s", status.Status, jobID, agentID)
		return
	}

	// Check if job is already in a terminal state (terminal state protection)
	if j.Status.IsTerminal() {
		log.Printf("JobStatus: job %s is already in terminal state %s, ignoring status update to %s from agent %s",
			jobID, j.Status, newStatus, agentID)
		return
	}

	// Handle status-specific logic
	switch newStatus {
	case job.StatusRunning:
		// Update to RUNNING
		// Save stdout/stderr if provided (for progress updates)
		stdout := status.Stdout
		stderr := status.Stderr
		if stdout != "" || stderr != "" {
			if err := g.jobStore.UpdateStdoutStderr(jobID, stdout, stderr); err != nil {
				log.Printf("Failed to update stdout/stderr for job %s: %v", jobID, err)
				// Continue anyway
			}
		}
		
		if err := g.jobStore.UpdateStatus(jobID, job.StatusRunning); err != nil {
			log.Printf("Failed to update job %s to RUNNING: %v", jobID, err)
			return
		}
		log.Printf("Job %s (attempt %d) is now RUNNING on agent %s", jobID, attemptID, agentID)

	case job.StatusSucceeded:
		// SUCCEEDED: output_key is optional (may be empty if command doesn't produce output file)
		outputKey := status.OutputKey
		
		// Save stdout and stderr
		stdout := status.Stdout
		stderr := status.Stderr
		if err := g.jobStore.UpdateStdoutStderr(jobID, stdout, stderr); err != nil {
			log.Printf("Failed to update stdout/stderr for job %s: %v", jobID, err)
			// Continue anyway - stdout/stderr update failure shouldn't fail the job
		}

		// If output_key is provided, validate it matches store.OutputKey (presigned mode)
		// Note: output_key is optional - if agent doesn't provide it, we allow SUCCEEDED
		// (command may only produce stdout without an output file)
		if outputKey != "" {
			// Fix 4: Strict validation - output_key must exactly equal store.OutputKey (presigned mode)
			if j.OutputKey == "" {
				log.Printf("JobStatus: job %s has no OutputKey in store, cannot validate presigned output_key, marking as FAILED", jobID)
				if err := g.jobStore.UpdateStatus(jobID, job.StatusFailed); err != nil {
					log.Printf("Failed to update job %s to FAILED: %v", jobID, err)
				} else {
					// Fix 5: Decrement RunningJobs on terminal state
					agentInfo, _ := g.registry.GetAgent(agentID)
					if agentInfo != nil && agentInfo.RunningJobs > 0 {
						g.registry.UpdateHeartbeat(agentID, agentInfo.Paused, agentInfo.RunningJobs-1)
					}
				}
				return
			}

			if outputKey != j.OutputKey {
				log.Printf("JobStatus: output_key mismatch for job %s: reported=%s, expected=%s, marking as FAILED (presigned mode requires exact match)", jobID, outputKey, j.OutputKey)
				// Mark as FAILED - do not update store.OutputKey to prevent pollution
				if err := g.jobStore.UpdateStatus(jobID, job.StatusFailed); err != nil {
					log.Printf("Failed to update job %s to FAILED: %v", jobID, err)
				} else {
					// Fix 5: Decrement RunningJobs on terminal state
					agentInfo, _ := g.registry.GetAgent(agentID)
					if agentInfo != nil && agentInfo.RunningJobs > 0 {
						g.registry.UpdateHeartbeat(agentID, agentInfo.Paused, agentInfo.RunningJobs-1)
					}
				}
				return
			}
		}

		// Update status to SUCCEEDED
		if err := g.jobStore.UpdateStatus(jobID, job.StatusSucceeded); err != nil {
			log.Printf("Failed to update job %s to SUCCEEDED: %v", jobID, err)
			return
		}
		if outputKey != "" {
			log.Printf("Job %s (attempt %d) SUCCEEDED on agent %s, output_key=%s", jobID, attemptID, agentID, outputKey)
		} else {
			log.Printf("Job %s (attempt %d) SUCCEEDED on agent %s (no output file, stdout only)", jobID, attemptID, agentID)
		}

		// Fix 5: Decrement RunningJobs on terminal state
		agentInfo, _ := g.registry.GetAgent(agentID)
		if agentInfo != nil && agentInfo.RunningJobs > 0 {
			g.registry.UpdateHeartbeat(agentID, agentInfo.Paused, agentInfo.RunningJobs-1)
		}

	case job.StatusFailed:
		// Update to FAILED
		message := status.Message
		
		// Save stdout and stderr (especially stderr for failed jobs)
		stdout := status.Stdout
		stderr := status.Stderr
		if err := g.jobStore.UpdateStdoutStderr(jobID, stdout, stderr); err != nil {
			log.Printf("Failed to update stdout/stderr for job %s: %v", jobID, err)
			// Continue anyway - stdout/stderr update failure shouldn't fail the status update
		}
		
		if message != "" {
			log.Printf("Job %s (attempt %d) FAILED on agent %s: %s", jobID, attemptID, agentID, message)
		} else {
			log.Printf("Job %s (attempt %d) FAILED on agent %s", jobID, attemptID, agentID)
		}
		if err := g.jobStore.UpdateStatus(jobID, job.StatusFailed); err != nil {
			log.Printf("Failed to update job %s to FAILED: %v", jobID, err)
			return
		}
		// Fix 5: Decrement RunningJobs on terminal state
		agentInfo, _ := g.registry.GetAgent(agentID)
		if agentInfo != nil && agentInfo.RunningJobs > 0 {
			g.registry.UpdateHeartbeat(agentID, agentInfo.Paused, agentInfo.RunningJobs-1)
		}

	case job.StatusCanceled, job.StatusLost:
		// Update to terminal state
		if !j.Status.CanTransitionTo(newStatus) {
			log.Printf("JobStatus: invalid transition from %s to %s for job %s from agent %s",
				j.Status, newStatus, jobID, agentID)
			return
		}
		if err := g.jobStore.UpdateStatus(jobID, newStatus); err != nil {
			log.Printf("Failed to update job %s to %s: %v", jobID, newStatus, err)
			return
		}
		log.Printf("Job %s (attempt %d) updated to %s on agent %s", jobID, attemptID, newStatus, agentID)
		// Fix 5: Decrement RunningJobs on terminal state
		agentInfo, _ := g.registry.GetAgent(agentID)
		if agentInfo != nil && agentInfo.RunningJobs > 0 {
			g.registry.UpdateHeartbeat(agentID, agentInfo.Paused, agentInfo.RunningJobs-1)
		}

	default:
		// For other statuses (ASSIGNED, RUNNING), try to update if transition is valid
		if !j.Status.CanTransitionTo(newStatus) {
			log.Printf("JobStatus: invalid transition from %s to %s for job %s from agent %s",
				j.Status, newStatus, jobID, agentID)
			return
		}
		if err := g.jobStore.UpdateStatus(jobID, newStatus); err != nil {
			log.Printf("Failed to update job %s to %s: %v", jobID, newStatus, err)
			return
		}
		log.Printf("Job %s (attempt %d) updated to %s on agent %s", jobID, attemptID, newStatus, agentID)
	}
}

// jobStatusFromProto converts protobuf JobStatusEnum to job.Status
func jobStatusFromProto(status control.JobStatusEnum) job.Status {
	switch status {
	case control.JobStatusEnum_JOB_STATUS_ASSIGNED:
		return job.StatusAssigned
	case control.JobStatusEnum_JOB_STATUS_RUNNING:
		return job.StatusRunning
	case control.JobStatusEnum_JOB_STATUS_SUCCEEDED:
		return job.StatusSucceeded
	case control.JobStatusEnum_JOB_STATUS_FAILED:
		return job.StatusFailed
	case control.JobStatusEnum_JOB_STATUS_CANCELED:
		return job.StatusCanceled
	case control.JobStatusEnum_JOB_STATUS_LOST:
		return job.StatusLost
	case control.JobStatusEnum_JOB_STATUS_UNKNOWN:
		fallthrough
	default:
		// Default to current job status or PENDING if unknown
		return job.StatusPending
	}
}

// SendMessage sends a message to an agent
func (g *Gateway) SendMessage(agentID string, envelope *control.Envelope) error {
	g.mu.RLock()
	agentConn, exists := g.connections[agentID]
	g.mu.RUnlock()

	if !exists {
		return ErrAgentNotFound
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}

	select {
	case agentConn.SendChan <- data:
		return nil
	default:
		return ErrSendBufferFull
	}
}

var (
	ErrAgentNotFound  = &Error{Message: "agent not found"}
	ErrSendBufferFull = &Error{Message: "send buffer full"}
)

type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// Helper to encode length-prefixed protobuf messages
func encodeMessage(msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// Prepend 4-byte length
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(data)))
	copy(buf[4:], data)
	return buf, nil
}
