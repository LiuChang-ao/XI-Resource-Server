package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	control "github.com/xiresource/proto/control"
	"google.golang.org/protobuf/proto"
)

// Client represents the agent client connection
type Client struct {
	serverURL         string
	agentID           string
	agentToken        string
	hostname          string
	maxConcurrency    int
	conn              *websocket.Conn
	heartbeatInterval time.Duration
	paused            bool
	runningJobs       int
	runningJobsMu     sync.Mutex
	stopChan          chan struct{}
	httpClient        *http.Client
}

// New creates a new agent client
func New(serverURL, agentID, agentToken string, maxConcurrency int) *Client {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	return &Client{
		serverURL:      serverURL,
		agentID:        agentID,
		agentToken:     agentToken,
		hostname:       hostname,
		maxConcurrency: maxConcurrency,
		stopChan:       make(chan struct{}),
		httpClient:     &http.Client{Timeout: 5 * time.Minute},
	}
}

// Connect connects to the server and starts the agent loop
func (c *Client) Connect() error {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(c.serverURL, nil)
	if err != nil {
		return err
	}
	c.conn = conn

	// Send Register message
	if err := c.sendRegister(); err != nil {
		conn.Close()
		return err
	}

	// Start read loop
	go c.readLoop()

	// Start heartbeat loop
	go c.heartbeatLoop()

	// Start job request loop (after registration succeeds)
	// Note: We'll start it after RegisterAck, but we can also start it here with backoff

	log.Printf("Agent %s connected to %s", c.agentID, c.serverURL)
	return nil
}

// Stop stops the client
func (c *Client) Stop() {
	close(c.stopChan)
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) sendRegister() error {
	envelope := &control.Envelope{
		AgentId:   c.agentID,
		RequestId: generateRequestID(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_Register{
			Register: &control.Register{
				AgentId:        c.agentID,
				AgentToken:     c.agentToken,
				Hostname:       c.hostname,
				MaxConcurrency: int32(c.maxConcurrency),
			},
		},
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Client) sendHeartbeat() error {
	envelope := &control.Envelope{
		AgentId:   c.agentID,
		RequestId: generateRequestID(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_Heartbeat{
			Heartbeat: &control.Heartbeat{
				AgentId:     c.agentID,
				Paused:      c.paused,
				RunningJobs: int32(c.runningJobs),
			},
		},
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Client) readLoop() {
	for {
		messageType, data, err := c.conn.ReadMessage()
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

		c.handleMessage(&envelope)
	}
}

func (c *Client) handleMessage(envelope *control.Envelope) {
	switch payload := envelope.Payload.(type) {
	case *control.Envelope_RegisterAck:
		c.handleRegisterAck(payload.RegisterAck)
	case *control.Envelope_HeartbeatAck:
		// Heartbeat acknowledged, no action needed
		log.Printf("Heartbeat acknowledged")
	case *control.Envelope_JobAssigned:
		c.handleJobAssigned(payload.JobAssigned)
	default:
		log.Printf("Unknown message type")
	}
}

func (c *Client) handleRegisterAck(ack *control.RegisterAck) {
	if ack.Success {
		if ack.HeartbeatIntervalSec > 0 {
			c.heartbeatInterval = time.Duration(ack.HeartbeatIntervalSec) * time.Second
		} else {
			c.heartbeatInterval = 20 * time.Second // Default
		}
		log.Printf("Registration successful, heartbeat interval: %v", c.heartbeatInterval)
		// Start job request loop after successful registration
		go c.jobRequestLoop()
	} else {
		log.Printf("Registration failed: %s", ack.Message)
	}
}

func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(20 * time.Second) // Default interval
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c.heartbeatInterval > 0 {
				ticker.Reset(c.heartbeatInterval)
			}
			if err := c.sendHeartbeat(); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
				return
			}
		case <-c.stopChan:
			return
		}
	}
}

// SetPaused sets the paused state
func (c *Client) SetPaused(paused bool) {
	c.paused = paused
}

// SetRunningJobs sets the number of running jobs
func (c *Client) SetRunningJobs(count int) {
	c.runningJobsMu.Lock()
	defer c.runningJobsMu.Unlock()
	c.runningJobs = count
}

// incrementRunningJobs increments running jobs counter (thread-safe)
func (c *Client) incrementRunningJobs() {
	c.runningJobsMu.Lock()
	defer c.runningJobsMu.Unlock()
	c.runningJobs++
}

// decrementRunningJobs decrements running jobs counter (thread-safe)
func (c *Client) decrementRunningJobs() {
	c.runningJobsMu.Lock()
	defer c.runningJobsMu.Unlock()
	if c.runningJobs > 0 {
		c.runningJobs--
	}
}

// getRunningJobs returns current running jobs count (thread-safe)
func (c *Client) getRunningJobs() int {
	c.runningJobsMu.Lock()
	defer c.runningJobsMu.Unlock()
	return c.runningJobs
}

// canAcceptJob checks if agent can accept a new job (thread-safe)
func (c *Client) canAcceptJob() bool {
	c.runningJobsMu.Lock()
	defer c.runningJobsMu.Unlock()
	return !c.paused && c.runningJobs < c.maxConcurrency
}

// Helper function to generate request ID (simple UUID v4-like)
func generateRequestID() string {
	// Simple implementation for MVP
	now := time.Now()
	return fmt.Sprintf("%d-%s", now.UnixNano(), randomHex(8))
}

func randomHex(n int) string {
	chars := "0123456789abcdef"
	result := make([]byte, n)
	seed := time.Now().UnixNano()
	charsLen := int64(len(chars))
	for i := range result {
		seed = seed*1103515245 + 12345 // Simple LCG
		idx := seed % charsLen
		if idx < 0 {
			idx = -idx // Make positive
		}
		result[i] = chars[idx]
	}
	return string(result)
}

// jobRequestLoop periodically requests jobs from the server
func (c *Client) jobRequestLoop() {
	backoff := 5 * time.Second // Initial backoff
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.stopChan:
			return
		case <-time.After(backoff):
			// Check if we can accept a new job
			if !c.canAcceptJob() {
				// If paused or at max capacity, wait longer
				backoff = min(backoff*2, maxBackoff)
				continue
			}

			// Reset backoff on successful request
			backoff = 5 * time.Second

			// Send RequestJob
			if err := c.sendRequestJob(); err != nil {
				log.Printf("Failed to send RequestJob: %v", err)
				backoff = min(backoff*2, maxBackoff)
			}
		}
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// sendRequestJob sends a RequestJob message to the server
func (c *Client) sendRequestJob() error {
	envelope := &control.Envelope{
		AgentId:   c.agentID,
		RequestId: generateRequestID(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_RequestJob{
			RequestJob: &control.RequestJob{
				AgentId: c.agentID,
			},
		},
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal RequestJob: %w", err)
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// handleJobAssigned processes a JobAssigned message
func (c *Client) handleJobAssigned(assigned *control.JobAssigned) {
	jobID := assigned.JobId
	if jobID == "" {
		log.Printf("JobAssigned missing job_id, skipping")
		return
	}

	attemptID := int(assigned.AttemptId)
	outputKey := assigned.OutputKey

	log.Printf("Received JobAssigned: job_id=%s, attempt_id=%d, output_key=%s", jobID, attemptID, outputKey)

	// Check if we can accept this job
	if !c.canAcceptJob() {
		log.Printf("Cannot accept job %s: paused=%v, running=%d, max=%d", jobID, c.paused, c.getRunningJobs(), c.maxConcurrency)
		// Report FAILED status
		c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, "Agent cannot accept job (paused or at capacity)", "")
		return
	}

	// Increment running jobs counter
	c.incrementRunningJobs()

	// Process job asynchronously
	go c.processJob(assigned)
}

// processJob processes a job: download input, compute, upload output, report status
func (c *Client) processJob(assigned *control.JobAssigned) {
	defer c.decrementRunningJobs()

	jobID := assigned.JobId
	attemptID := int(assigned.AttemptId)
	outputKey := assigned.OutputKey

	// Report RUNNING status
	c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_RUNNING, fmt.Sprintf("Processing job"), "")

	// Download input
	inputAccess := assigned.InputDownload
	if inputAccess == nil {
		log.Printf("JobAssigned missing input_download for job %s", jobID)
		c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, "Missing input_download", "")
		return
	}

	// Get presigned URL from OSSAccess
	var inputURL string
	switch auth := inputAccess.Auth.(type) {
	case *control.OSSAccess_PresignedUrl:
		inputURL = auth.PresignedUrl
	case *control.OSSAccess_Sts:
		// STS not implemented in M1
		log.Printf("STS not supported in M1 for job %s", jobID)
		c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, "STS not supported", "")
		return
	default:
		log.Printf("Invalid input_download auth type for job %s", jobID)
		c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, "Invalid input_download", "")
		return
	}

	// Check if command is provided
	if assigned.Command == "" {
		log.Printf("JobAssigned missing command for job %s", jobID)
		c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, "Command is required", "")
		return
	}

	// Download input to temporary file (preserve extension from input_key)
	inputFile, err := c.downloadInputToFile(inputURL, jobID, assigned.InputKey)
	if err != nil {
		log.Printf("Failed to download input for job %s: %v", jobID, err)
		c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, fmt.Sprintf("Download failed: %v", err), "")
		return
	}
	defer func() {
		if err := os.Remove(inputFile); err != nil {
			log.Printf("Warning: Failed to cleanup input file %s: %v", inputFile, err)
		}
	}()

	// Create output file path
	outputFile := filepath.Join(os.TempDir(), fmt.Sprintf("job_%s_output", jobID))
	defer func() {
		if err := os.Remove(outputFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to cleanup output file %s: %v", outputFile, err)
		}
	}()

	log.Printf("Executing command for job %s: %s", jobID, assigned.Command)

	// Execute command
	cmdResult, err := c.executeCommand(assigned.Command, inputFile, outputFile)
	if err != nil {
		log.Printf("Failed to execute command for job %s: %v", jobID, err)
		// Report FAILED with stderr (cmdResult may still contain stdout/stderr even on error)
		if cmdResult != nil {
			c.reportJobStatusWithOutput(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, 
				fmt.Sprintf("Command execution failed: %v", err), "", cmdResult.Stdout, cmdResult.Stderr)
		} else {
			c.reportJobStatus(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, 
				fmt.Sprintf("Command execution failed: %v", err), "")
		}
		return
	}

	// Upload output if output file exists and has data
	outputKeyToReport := ""
	if cmdResult.HasOutputFile && len(cmdResult.OutputData) > 0 {
		outputAccess := assigned.OutputUpload
		if outputAccess == nil {
			log.Printf("JobAssigned missing output_upload for job %s", jobID)
			c.reportJobStatusWithOutput(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, 
				"Missing output_upload", "", cmdResult.Stdout, cmdResult.Stderr)
			return
		}

		// Get presigned URL from OSSAccess
		var outputURL string
		switch auth := outputAccess.Auth.(type) {
		case *control.OSSAccess_PresignedUrl:
			outputURL = auth.PresignedUrl
		case *control.OSSAccess_Sts:
			// STS not implemented in M1
			log.Printf("STS not supported in M1 for job %s", jobID)
			c.reportJobStatusWithOutput(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, 
				"STS not supported", "", cmdResult.Stdout, cmdResult.Stderr)
			return
		default:
			log.Printf("Invalid output_upload auth type for job %s", jobID)
			c.reportJobStatusWithOutput(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, 
				"Invalid output_upload", "", cmdResult.Stdout, cmdResult.Stderr)
			return
		}

		// Upload output
		if err := c.uploadOutput(outputURL, cmdResult.OutputData); err != nil {
			log.Printf("Failed to upload output for job %s: %v", jobID, err)
			c.reportJobStatusWithOutput(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_FAILED, 
				fmt.Sprintf("Upload failed: %v", err), "", cmdResult.Stdout, cmdResult.Stderr)
			return
		}

		log.Printf("Uploaded output for job %s to %s", jobID, outputKey)
		outputKeyToReport = outputKey
	} else {
		log.Printf("Job %s completed without output file (stdout only or no output)", jobID)
	}

	// Report SUCCEEDED with output_key (may be empty if no output file) and stdout/stderr
	c.reportJobStatusWithOutput(jobID, attemptID, control.JobStatusEnum_JOB_STATUS_SUCCEEDED, "", 
		outputKeyToReport, cmdResult.Stdout, cmdResult.Stderr)
}

// downloadInput downloads input from presigned URL
func (c *Client) downloadInput(url string) ([]byte, int64, string, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, 0, "", fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, "", fmt.Errorf("HTTP GET returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, "", fmt.Errorf("failed to read response body: %w", err)
	}

	inputSize := int64(len(data))
	hash := sha256.Sum256(data)
	inputSHA256 := fmt.Sprintf("%x", hash)

	return data, inputSize, inputSHA256, nil
}

// stubCompute generates deterministic output based on input
type StubOutput struct {
	JobID      string `json:"job_id"`
	AttemptID  int    `json:"attempt_id"`
	InputSize  int64  `json:"input_size"`
	InputSHA256 string `json:"input_sha256"`
	Timestamp  int64  `json:"timestamp"`
}

// stubCompute generates deterministic JSON output
// NOTE: This function is kept for backward compatibility but is no longer used
// when command execution is enabled
func (c *Client) stubCompute(jobID string, attemptID int, inputSize int64, inputSHA256 string) ([]byte, error) {
	output := StubOutput{
		JobID:      jobID,
		AttemptID:  attemptID,
		InputSize:  inputSize,
		InputSHA256: inputSHA256,
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal output: %w", err)
	}

	return data, nil
}

// downloadInputToFile downloads input from presigned URL to a temporary file
// It preserves the file extension from input_key if available
func (c *Client) downloadInputToFile(url, jobID, inputKey string) (string, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP GET returned status %d", resp.StatusCode)
	}

	// Extract extension from input_key if available
	extension := ""
	if inputKey != "" {
		ext := filepath.Ext(inputKey)
		if ext != "" {
			extension = ext
		}
	}

	// Create temporary file with extension preserved
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("job_%s_input%s", jobID, extension))
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer f.Close()

	// Write to file
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("failed to write to file: %w", err)
	}

	return tmpFile, nil
}

// CommandResult contains the result of command execution
type CommandResult struct {
	OutputData []byte // Output file content (if exists)
	Stdout     string // Command stdout
	Stderr     string // Command stderr
	HasOutputFile bool // Whether output file exists
}

// executeCommand executes the given command with input/output file placeholders
func (c *Client) executeCommand(command string, inputFile, outputFile string) (*CommandResult, error) {
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Replace placeholders
	cmdStr := strings.ReplaceAll(command, "{input}", inputFile)
	cmdStr = strings.ReplaceAll(cmdStr, "{output}", outputFile)

	log.Printf("Executing command: %s", cmdStr)

	// Parse command (Windows: use cmd.exe /C, Linux: use sh -c)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/C", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	// Set timeout (30 minutes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	startTime := time.Now()
	err := cmd.Run()
	executionTime := time.Since(startTime)

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if err != nil {
		log.Printf("Command execution failed after %v: %v, stderr: %s", executionTime, err, stderrStr)
		return &CommandResult{
			OutputData: nil,
			Stdout:     truncateString(stdoutStr, 10000), // Limit to 10KB
			Stderr:     truncateString(stderrStr, 10000), // Limit to 10KB
			HasOutputFile: false,
		}, fmt.Errorf("command execution failed: %v", err)
	}

	log.Printf("Command executed successfully in %v, stdout: %s", executionTime, stdoutStr)

	// Try to read output file
	outputData, err := os.ReadFile(outputFile)
	hasOutputFile := err == nil

	result := &CommandResult{
		OutputData: outputData,
		Stdout:     truncateString(stdoutStr, 10000), // Limit to 10KB
		Stderr:     truncateString(stderrStr, 10000), // Limit to 10KB
		HasOutputFile: hasOutputFile,
	}

	// If output file doesn't exist, use stdout as output data
	if !hasOutputFile {
		if stdout.Len() > 0 {
			log.Printf("Output file not found, using stdout as output")
			result.OutputData = stdout.Bytes()
		} else {
			// No output file and no stdout - this is OK for commands that don't produce output
			result.OutputData = nil
		}
	}

	return result, nil
}

// truncateString truncates a string to maxLen, appending "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

// uploadOutput uploads output to presigned URL
func (c *Client) uploadOutput(url string, data []byte) error {
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(data))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP PUT failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("HTTP PUT returned status %d", resp.StatusCode)
	}

	return nil
}

// reportJobStatus reports job status to the server (backward compatible)
func (c *Client) reportJobStatus(jobID string, attemptID int, status control.JobStatusEnum, message, outputKey string) {
	c.reportJobStatusWithOutput(jobID, attemptID, status, message, outputKey, "", "")
}

// reportJobStatusWithOutput reports job status to the server with stdout/stderr
func (c *Client) reportJobStatusWithOutput(jobID string, attemptID int, status control.JobStatusEnum, message, outputKey, stdout, stderr string) {
	jobStatus := &control.JobStatus{
		JobId:     jobID,
		AttemptId: int32(attemptID),
		Status:    status,
		Message:   message,
		OutputKey: outputKey,
		Stdout:    stdout,
		Stderr:    stderr,
	}

	envelope := &control.Envelope{
		AgentId:   c.agentID,
		RequestId: generateRequestID(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &control.Envelope_JobStatus{
			JobStatus: jobStatus,
		},
	}

	data, err := proto.Marshal(envelope)
	if err != nil {
		log.Printf("Failed to marshal JobStatus: %v", err)
		return
	}

	if c.conn != nil {
		if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			log.Printf("Failed to send JobStatus: %v", err)
			return
		}
		log.Printf("Reported job %s (attempt %d) status: %v", jobID, attemptID, status)
	} else {
		// In tests, conn might be nil
		log.Printf("Would report job %s (attempt %d) status: %v (conn is nil)", jobID, attemptID, status)
	}
}
