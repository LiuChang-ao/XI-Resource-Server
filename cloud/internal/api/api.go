package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xiresource/cloud/internal/job"
	"github.com/xiresource/cloud/internal/queue"
	"github.com/xiresource/cloud/internal/registry"
)

// AgentInfo represents an online agent (API response format)
type AgentInfo struct {
	AgentID        string `json:"agent_id"`
	Hostname       string `json:"hostname"`
	MaxConcurrency int    `json:"max_concurrency"`
	Paused         bool   `json:"paused"`
	RunningJobs    int    `json:"running_jobs"`
	LastHeartbeat  string `json:"last_heartbeat"` // ISO 8601 format
	ConnectedAt    string `json:"connected_at"`   // ISO 8601 format
}

// Handler handles HTTP API requests
type Handler struct {
	registry *registry.Registry
	jobStore job.Store
	queue    queue.Queue
}

// New creates a new API handler
func New(reg *registry.Registry, jobStore job.Store, jobQueue queue.Queue) *Handler {
	return &Handler{
		registry: reg,
		jobStore: jobStore,
		queue:    jobQueue,
	}
}

const (
	// MaxRequestBodySize is the maximum allowed body size for POST /jobs (1MB)
	MaxRequestBodySize = 1 * 1024 * 1024 // 1MB
)

// HandleAgentsOnline returns the list of online agents
func (h *Handler) HandleAgentsOnline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := h.registry.GetOnline()

	// Convert to API format
	apiAgents := make([]AgentInfo, len(agents))
	for i, agent := range agents {
		apiAgents[i] = AgentInfo{
			AgentID:        agent.AgentID,
			Hostname:       agent.Hostname,
			MaxConcurrency: agent.MaxConcurrency,
			Paused:         agent.Paused,
			RunningJobs:    agent.RunningJobs,
			LastHeartbeat:  agent.LastHeartbeat.Format("2006-01-02T15:04:05Z07:00"),
			ConnectedAt:    agent.ConnectedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(apiAgents); err != nil {
		log.Printf("Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleHealth returns health status
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// CreateJobRequest represents the request body for creating a job
type CreateJobRequest struct {
	InputBucket       string            `json:"input_bucket"`
	InputKey          string            `json:"input_key"`
	OutputBucket      string            `json:"output_bucket"`
	OutputKey         string            `json:"output_key,omitempty"`          // Optional: specific output key
	OutputPrefix      string            `json:"output_prefix,omitempty"`       // Optional: output prefix (defaults to jobs/{job_id}/{attempt_id}/)
	OutputExtension   string            `json:"output_extension,omitempty"`    // Optional: output file extension (e.g., "json", "txt", "bin", default: "bin")
	AttemptID         int               `json:"attempt_id,omitempty"`          // Optional: defaults to 1
	Command           string            `json:"command,omitempty"`             // Optional: command to execute (e.g., "python C:/scripts/analyze.py {input} {output}")
	JobType           string            `json:"job_type,omitempty"`            // Optional: COMMAND or FORWARD_HTTP
	ForwardURL        string            `json:"forward_url,omitempty"`         // Optional: local service URL for forward jobs
	ForwardMethod     string            `json:"forward_method,omitempty"`      // Optional: HTTP method for forward jobs
	ForwardHeaders    map[string]string `json:"forward_headers,omitempty"`     // Optional: headers for forward jobs
	ForwardBody       string            `json:"forward_body,omitempty"`        // Optional: raw body for forward jobs
	ForwardTimeoutSec int               `json:"forward_timeout_sec,omitempty"` // Optional: timeout for forward jobs (seconds)
	InputForwardMode  string            `json:"input_forward_mode,omitempty"`  // Optional: URL or LOCAL_FILE
}

// CreateJobResponse represents the response for creating a job
type CreateJobResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// HandleCreateJob handles POST /api/jobs
// Security guards:
// - Rejects multipart/form-data (prevents file upload bypass)
// - Limits body size to 1MB (enforces OSS-only data plane)
// - Only accepts OSS keys in JSON format
func (h *Handler) HandleCreateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Guard 1: Reject multipart/form-data to prevent file upload bypass
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		http.Error(w, "multipart/form-data is not allowed. Use OSS keys in JSON format only.", http.StatusUnsupportedMediaType)
		return
	}

	// Guard 2: Enforce application/json content type
	if contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		http.Error(w, "Content-Type must be application/json. Only OSS keys are accepted, not file content.", http.StatusUnsupportedMediaType)
		return
	}

	// Guard 3: Limit body size to prevent large file uploads
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	// Read body with size limit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, fmt.Sprintf("Request body exceeds maximum size of %d bytes. Only OSS keys are accepted, not file content.", MaxRequestBodySize), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Guard 4: Ensure body is not empty
	if len(body) == 0 {
		http.Error(w, "Request body is required", http.StatusBadRequest)
		return
	}

	// Parse JSON
	var req CreateJobRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v. Only OSS keys are accepted, not file content.", err), http.StatusBadRequest)
		return
	}

	// Validate that we have OSS keys (not file content)
	// Input is optional - jobs can run without input files (e.g., scheduled tasks, pure computation)
	// If input_bucket is provided, input_key must also be provided (and vice versa)
	if (req.InputBucket == "" && req.InputKey != "") || (req.InputBucket != "" && req.InputKey == "") {
		http.Error(w, "input_bucket and input_key must both be provided or both be empty (OSS keys only, not file content)", http.StatusBadRequest)
		return
	}
	// Output bucket is optional - if not provided, gateway will use OSS provider's default bucket
	// This allows jobs that only produce stdout/stderr without output files

	// Generate job ID
	jobID := uuid.New().String()

	// Set defaults
	attemptID := req.AttemptID
	if attemptID < 1 {
		attemptID = 1
	}

	// Validate command length if provided
	const maxCommandLength = 8192 // 8KB should be sufficient for most command lines
	if len(req.Command) > maxCommandLength {
		http.Error(w, fmt.Sprintf("command exceeds maximum length of %d characters", maxCommandLength), http.StatusBadRequest)
		return
	}

	// Normalize and validate job type
	jobType := strings.ToUpper(strings.TrimSpace(req.JobType))
	if jobType == "" {
		jobType = string(job.JobTypeCommand)
	}
	if jobType != string(job.JobTypeCommand) && jobType != string(job.JobTypeForwardHTTP) {
		http.Error(w, "job_type must be COMMAND or FORWARD_HTTP", http.StatusBadRequest)
		return
	}

	// Validate forward job fields
	inputForwardMode := strings.ToUpper(strings.TrimSpace(req.InputForwardMode))
	if inputForwardMode != "" && inputForwardMode != string(job.InputForwardModeURL) && inputForwardMode != string(job.InputForwardModeLocalFile) {
		http.Error(w, "input_forward_mode must be URL or LOCAL_FILE", http.StatusBadRequest)
		return
	}
	if jobType == string(job.JobTypeForwardHTTP) {
		if strings.TrimSpace(req.ForwardURL) == "" {
			http.Error(w, "forward_url is required for FORWARD_HTTP job_type", http.StatusBadRequest)
			return
		}
	}

	// Set default output extension if not provided
	outputExtension := req.OutputExtension
	if outputExtension == "" {
		outputExtension = "bin" // Default extension
	}
	// Remove leading dot if present
	if strings.HasPrefix(outputExtension, ".") {
		outputExtension = outputExtension[1:]
	}

	// Create job
	forwardHeadersJSON := ""
	if len(req.ForwardHeaders) > 0 {
		headersData, err := json.Marshal(req.ForwardHeaders)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid forward_headers: %v", err), http.StatusBadRequest)
			return
		}
		forwardHeadersJSON = string(headersData)
	}

	newJob := &job.Job{
		JobID:           jobID,
		CreatedAt:       time.Now(),
		Status:          job.StatusPending,
		InputBucket:     req.InputBucket,
		InputKey:        req.InputKey,
		OutputBucket:    req.OutputBucket,
		OutputKey:       req.OutputKey,
		OutputPrefix:    req.OutputPrefix,
		OutputExtension: outputExtension,
		AttemptID:       attemptID,
		AssignedAgentID: "",
		LeaseID:         "",
		LeaseDeadline:   nil,
		Command:         req.Command,
		Stdout:          "",
		Stderr:          "",
		JobType:         job.JobType(jobType),
		ForwardURL:      strings.TrimSpace(req.ForwardURL),
		ForwardMethod:   strings.TrimSpace(req.ForwardMethod),
		ForwardHeaders:  forwardHeadersJSON,
		ForwardBody:     req.ForwardBody,
		ForwardTimeout:  req.ForwardTimeoutSec,
		InputForward:    job.InputForwardMode(inputForwardMode),
	}

	// Ensure output prefix follows pattern
	newJob.EnsureOutputPrefix()

	// Validate job before persisting
	if err := newJob.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid job: %v", err), http.StatusBadRequest)
		return
	}

	// Persist job to database
	if err := h.jobStore.Create(newJob); err != nil {
		log.Printf("Failed to create job: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create job: %v", err), http.StatusInternalServerError)
		return
	}

	// Enqueue job to Redis for scheduler
	if h.queue != nil {
		ctx := r.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		if err := h.queue.Enqueue(ctx, jobID); err != nil {
			// Log error but don't fail the request - job is already persisted
			// Scheduler can recover from database if needed
			log.Printf("Warning: Failed to enqueue job %s to Redis: %v (job was created in database)", jobID, err)
		} else {
			log.Printf("Job %s enqueued to Redis queue", jobID)
		}
	} else {
		log.Printf("Warning: No queue configured, job %s created but not enqueued", jobID)
	}

	response := CreateJobResponse{
		JobID:     newJob.JobID,
		Status:    string(newJob.Status),
		CreatedAt: newJob.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// HandleGetJob handles GET /api/jobs/{job_id}
func (h *Handler) HandleGetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job_id from path
	path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}

	jobID := path
	if _, err := uuid.Parse(jobID); err != nil {
		http.Error(w, "Invalid job_id format", http.StatusBadRequest)
		return
	}

	j, err := h.jobStore.Get(jobID)
	if err == job.ErrJobNotFound {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Failed to get job: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(j); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// HandleListJobs handles GET /api/jobs
func (h *Handler) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	limit := 100 // default
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var statusFilter *job.Status
	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		s := job.Status(statusStr)
		if s.IsValid() {
			statusFilter = &s
		}
	}

	jobs, err := h.jobStore.List(limit, offset, statusFilter)
	if err != nil {
		log.Printf("Failed to list jobs: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jobs); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}
