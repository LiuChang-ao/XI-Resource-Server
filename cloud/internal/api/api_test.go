package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/xiresource/cloud/internal/job"
	"github.com/xiresource/cloud/internal/queue"
	"github.com/xiresource/cloud/internal/registry"
)

func init() {
	// Load .env file for tests
	// Try to load from current directory (cloud/) and parent directories
	envPaths := []string{
		".env",           // cloud/.env
		"../.env",        // project root .env
		"../../.env",     // if running from subdirectory
	}
	
	for _, path := range envPaths {
		if err := godotenv.Load(path); err == nil {
			// Successfully loaded, no need to try other paths
			return
		}
	}
	// If no .env file found, tests will use system environment variables or defaults
}

// Test helpers

func setupTestServer(t *testing.T, withQueue bool) (*httptest.Server, *Handler, func()) {
	return setupTestServerWithQueue(t, nil, withQueue)
}

// setupTestServerWithQueue allows specifying a custom queue (for testing)
func setupTestServerWithQueue(t *testing.T, customQueue queue.Queue, useRedisIfAvailable bool) (*httptest.Server, *Handler, func()) {
	// Create job store (in-memory SQLite for testing)
	jobStore, err := job.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create job store: %v", err)
	}

	// Create registry
	reg := registry.New()

	// Create queue
	var jobQueue queue.Queue
	var redisClient interface {
		Close() error
	}

	if customQueue != nil {
		// Use provided queue (e.g., InMemoryQueue for testing)
		jobQueue = customQueue
	} else if useRedisIfAvailable {
		// Try to connect to Redis if available
		redisConfig := queue.LoadConfig()
		if redisConfig.IsConfigured() {
			client, err := queue.NewRedisClient(redisConfig)
			if err == nil {
				redisClient = client
				jobQueue = queue.NewRedisQueue(client)
			}
		}
	}

	// If still no queue, use in-memory queue for stable testing
	if jobQueue == nil && useRedisIfAvailable {
		jobQueue = queue.NewInMemoryQueue()
	}

	// Create API handler
	handler := New(reg, jobStore, jobQueue)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handler.HandleCreateJob(w, r)
		case http.MethodGet:
			handler.HandleListJobs(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	server := httptest.NewServer(mux)

	cleanup := func() {
		server.Close()
		jobStore.Close()
		if redisClient != nil {
			redisClient.Close()
		}
	}

	return server, handler, cleanup
}

// Test cases

func TestHandleCreateJob_Success(t *testing.T) {
	// Use InMemoryQueue for stable testing without Redis dependency
	inMemoryQueue := queue.NewInMemoryQueue()
	server, _, cleanup := setupTestServerWithQueue(t, inMemoryQueue, false)
	defer cleanup()

	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		InputKey:     fmt.Sprintf("inputs/job-%s/data.zip", time.Now().Format("20060102150405")),
		OutputBucket: "test-bucket",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 201, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var response CreateJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.JobID == "" {
		t.Error("Expected job_id in response, got empty")
	}

	if response.Status != "PENDING" {
		t.Errorf("Expected status PENDING, got %s", response.Status)
	}

	// Verify job was enqueued (using in-memory queue)
	ctx := context.Background()
	size, err := inMemoryQueue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if size != 1 {
		t.Errorf("Expected queue size 1, got %d", size)
	}

	peeked, err := inMemoryQueue.Peek(ctx)
	if err != nil {
		t.Fatalf("Failed to peek queue: %v", err)
	}
	if peeked != response.JobID {
		t.Errorf("Expected job ID %s in queue, got %s", response.JobID, peeked)
	}

	t.Logf("Created job: %s", response.JobID)
}

func TestHandleCreateJob_RejectMultipart(t *testing.T) {
	server, _, cleanup := setupTestServer(t, false)
	defer cleanup()

	// Create a real multipart body with boundary and a file part
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)

	// Add a file field (simulating file upload)
	fileWriter, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	fileWriter.Write([]byte("test file content"))

	// Add other fields
	writer.WriteField("input_bucket", "test-bucket")
	writer.WriteField("input_key", "test.txt")
	writer.WriteField("output_bucket", "test-bucket")

	writer.Close()

	req, err := http.NewRequest("POST", server.URL+"/api/jobs", &multipartBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	// Set proper multipart Content-Type with boundary
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 415, got %d: %s", resp.StatusCode, string(body))
		return
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "multipart/form-data is not allowed") {
		t.Errorf("Expected error message about multipart, got: %s", bodyStr)
	}
}

func TestHandleCreateJob_RejectLargeBody(t *testing.T) {
	server, _, cleanup := setupTestServer(t, false)
	defer cleanup()

	// Create a body larger than 1MB
	// Use output_prefix field (which is allowed and optional) to ensure
	// size guard is tested before field validation, and that even if
	// DisallowUnknownFields is enabled in future, this test still works
	largeData := strings.Repeat("x", int(1.5*1024*1024))
	reqBodyMap := map[string]interface{}{
		"input_bucket":  "test-bucket",
		"input_key":     "test.txt",
		"output_bucket": "test-bucket",
		"output_prefix": largeData, // Use allowed field to avoid 400 from unknown field
	}

	jsonData, err := json.Marshal(reqBodyMap)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 413, got %d: %s", resp.StatusCode, string(body))
		return
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "exceeds maximum size") {
		t.Errorf("Expected error message about size limit, got: %s", bodyStr)
	}
}

func TestHandleCreateJob_RejectNonJSON(t *testing.T) {
	server, _, cleanup := setupTestServer(t, false)
	defer cleanup()

	bodyText := `{"input_bucket":"test","input_key":"test","output_bucket":"test"}`

	req, err := http.NewRequest("POST", server.URL+"/api/jobs", strings.NewReader(bodyText))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 415, got %d: %s", resp.StatusCode, string(body))
		return
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "application/json") {
		t.Errorf("Expected error message about JSON Content-Type, got: %s", bodyStr)
	}
}

func TestHandleCreateJob_ValidateRequiredFields(t *testing.T) {
	server, _, cleanup := setupTestServer(t, false)
	defer cleanup()

	// Test: Missing input_key but has input_bucket (should fail)
	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		OutputBucket: "test-bucket",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 400, got %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "input_bucket and input_key must both be provided") {
		t.Errorf("Expected error message about input_bucket and input_key, got: %s", bodyStr)
	}

	// Test: Missing input_bucket but has input_key (should fail)
	reqBody2 := CreateJobRequest{
		InputKey:     "test-key",
		OutputBucket: "test-bucket",
	}

	jsonData2, err := json.Marshal(reqBody2)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp2, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData2))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("Expected status 400, got %d: %s", resp2.StatusCode, string(body))
	}

	body2, _ := io.ReadAll(resp2.Body)
	bodyStr2 := string(body2)
	if !strings.Contains(bodyStr2, "input_bucket and input_key must both be provided") {
		t.Errorf("Expected error message about input_bucket and input_key, got: %s", bodyStr2)
	}
}

// TestHandleCreateJob_ValidateInputFields_JSONMissingField tests the case where
// JSON request doesn't include input_key field at all (not just empty string)
func TestHandleCreateJob_ValidateInputFields_JSONMissingField(t *testing.T) {
	server, _, cleanup := setupTestServer(t, false)
	defer cleanup()

	// Test: JSON with input_bucket but missing input_key field entirely
	// This simulates a request like: {"input_bucket": "test", "output_bucket": "test"}
	// When input_key is missing from JSON, Go unmarshals it as empty string ""
	reqBodyMap := map[string]interface{}{
		"input_bucket":  "test-bucket",
		"output_bucket": "test-bucket",
		// Note: input_key is intentionally omitted
	}

	jsonData, err := json.Marshal(reqBodyMap)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should fail because input_bucket is provided but input_key is empty (missing from JSON)
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 400, got %d: %s", resp.StatusCode, string(body))
		return
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "input_bucket and input_key must both be provided") {
		t.Errorf("Expected error message about input_bucket and input_key, got: %s", bodyStr)
	}
}

func TestHandleCreateJob_NoInputFile(t *testing.T) {
	// Use InMemoryQueue for stable testing without Redis dependency
	inMemoryQueue := queue.NewInMemoryQueue()
	server, _, cleanup := setupTestServerWithQueue(t, inMemoryQueue, false)
	defer cleanup()

	// Create job without input files
	reqBody := CreateJobRequest{
		OutputBucket: "test-bucket",
		Command:      "echo Hello World",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 201, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var response CreateJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.JobID == "" {
		t.Error("Expected job_id in response, got empty")
	}

	if response.Status != "PENDING" {
		t.Errorf("Expected status PENDING, got %s", response.Status)
	}

	// Verify job was enqueued
	ctx := context.Background()
	size, err := inMemoryQueue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if size != 1 {
		t.Errorf("Expected queue size 1, got %d", size)
	}

	peeked, err := inMemoryQueue.Peek(ctx)
	if err != nil {
		t.Fatalf("Failed to peek queue: %v", err)
	}
	if peeked != response.JobID {
		t.Errorf("Expected job ID %s in queue, got %s", response.JobID, peeked)
	}

	t.Logf("Created job without input: %s", response.JobID)
}

// TestHandleCreateJob_NoInputNoOutput tests creating a job with neither input nor output files
// This simulates jobs that only produce stdout/stderr without any file I/O
func TestHandleCreateJob_NoInputNoOutput(t *testing.T) {
	// Use InMemoryQueue for stable testing without Redis dependency
	inMemoryQueue := queue.NewInMemoryQueue()
	server, _, cleanup := setupTestServerWithQueue(t, inMemoryQueue, false)
	defer cleanup()

	// Create job without input files and without output bucket
	// JSON with null values should be handled correctly
	reqBodyMap := map[string]interface{}{
		"input_bucket":     nil,
		"input_key":        nil,
		"output_bucket":    nil,
		"output_key":       nil,
		"output_prefix":    nil,
		"output_extension": "json",
		"attempt_id":       1,
		"command":          "echo Hello World",
	}

	jsonData, err := json.Marshal(reqBodyMap)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 201, got %d: %s", resp.StatusCode, string(body))
		return
	}

	var response CreateJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.JobID == "" {
		t.Error("Expected job_id in response, got empty")
	}

	if response.Status != "PENDING" {
		t.Errorf("Expected status PENDING, got %s", response.Status)
	}

	// Verify job was enqueued
	ctx := context.Background()
	size, err := inMemoryQueue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if size != 1 {
		t.Errorf("Expected queue size 1, got %d", size)
	}

	t.Logf("Created job without input and output: %s", response.JobID)
}

func TestHandleCreateJob_RejectEmptyBody(t *testing.T) {
	server, _, cleanup := setupTestServer(t, false)
	defer cleanup()

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", strings.NewReader(""))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestHandleCreateJob_RedisEnqueue(t *testing.T) {
	// Skip if Redis is not available
	redisConfig := queue.LoadConfig()
	if !redisConfig.IsConfigured() {
		t.Skip("Redis not configured, skipping Redis queue test")
	}

	redisClient, err := queue.NewRedisClient(redisConfig)
	if err != nil {
		t.Skipf("Cannot connect to Redis: %v, skipping Redis queue test", err)
	}
	defer redisClient.Close()

	// Use a test-specific queue key to avoid polluting the global queue
	// Use timestamp + random suffix to ensure uniqueness across parallel test runs
	testQueueKey := fmt.Sprintf("jobs:test:%d", time.Now().UnixNano())
	testQueue := queue.NewRedisQueueWithKey(redisClient, testQueueKey)
	ctx := context.Background()

	// Ensure test queue is clean (delete the key if it exists)
	redisClient.Del(ctx, testQueueKey)

	// Create server with test-specific Redis queue
	jobStore, err := job.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create job store: %v", err)
	}
	defer jobStore.Close()

	// Cleanup: remove test queue key after test
	defer func() {
		redisClient.Del(ctx, testQueueKey)
	}()

	reg := registry.New()
	jobQueue := queue.NewRedisQueueWithKey(redisClient, testQueueKey)
	handler := New(reg, jobStore, jobQueue)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.HandleCreateJob(w, r)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a job
	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		InputKey:     "inputs/test/data.zip",
		OutputBucket: "test-bucket",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(server.URL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(body))
	}

	var response CreateJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Poll for job to appear in queue (with timeout)
	// Enqueue is synchronous, but we poll to be defensive against timing issues
	maxWait := 2 * time.Second
	pollInterval := 50 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	var found bool
	for time.Now().Before(deadline) {
		size, err := testQueue.Size(ctx)
		if err != nil {
			t.Fatalf("Failed to get queue size: %v", err)
		}

		if size > 0 {
			peeked, err := testQueue.Peek(ctx)
			if err != nil {
				// Queue might be empty between size check and peek
				time.Sleep(pollInterval)
				continue
			}

			// Peek already returns unmarshaled job ID (queue.Peek handles unmarshal internally)
			if peeked == response.JobID {
				found = true
				break
			}
		}

		time.Sleep(pollInterval)
	}

	if !found {
		size, _ := testQueue.Size(ctx)
		peeked, _ := testQueue.Peek(ctx)
		t.Errorf("Job ID %s not found in Redis queue after %v (queue size: %d, peeked: %s)", 
			response.JobID, maxWait, size, peeked)
	}
}

// Integration test using docker exec (optional, requires Redis container)
// Only runs if RUN_INTEGRATION=1 environment variable is set
func TestRedisQueueIntegration(t *testing.T) {
	// Require explicit opt-in via environment variable
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("Skipping integration test (set RUN_INTEGRATION=1 to run)")
	}

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if docker and redis container are available
	cmd := exec.Command("docker", "exec", "redis", "redis-cli", "PING")
	err := cmd.Run()
	if err != nil {
		t.Skip("Redis container not available, skipping integration test")
	}

	// Use test-specific queue key to avoid polluting shared environment
	testQueueKey := fmt.Sprintf("jobs:test:integration:%d", time.Now().UnixNano())
	ctx := context.Background()

	redisConfig := queue.LoadConfig()
	redisConfig.Host = "localhost"
	redisConfig.Port = 6379

	redisClient, err := queue.NewRedisClient(redisConfig)
	if err != nil {
		t.Skipf("Cannot connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Cleanup: ensure test queue key is deleted after test
	defer func() {
		redisClient.Del(ctx, testQueueKey)
	}()

	testQueue := queue.NewRedisQueueWithKey(redisClient, testQueueKey)

	// Test enqueue
	testJobID := fmt.Sprintf("test-job-%d", time.Now().UnixNano())
	err = testQueue.Enqueue(ctx, testJobID)
	if err != nil {
		t.Fatalf("Failed to enqueue: %v", err)
	}

	// Test size
	size, err := testQueue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get size: %v", err)
	}
	if size != 1 {
		t.Errorf("Expected queue size 1, got %d", size)
	}

	// Test peek
	peeked, err := testQueue.Peek(ctx)
	if err != nil {
		t.Fatalf("Failed to peek: %v", err)
	}

	// Peek already returns unmarshaled job ID (queue.Peek handles unmarshal internally)
	if peeked != testJobID {
		t.Errorf("Peeked job ID %s does not match expected %s", peeked, testJobID)
	}

	// Test dequeue
	dequeued, err := testQueue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Failed to dequeue: %v", err)
	}

	// Dequeue also returns unmarshaled job ID (queue.Dequeue handles unmarshal internally)
	if dequeued != testJobID {
		t.Errorf("Dequeued job ID %s does not match expected %s", dequeued, testJobID)
	}

	// Verify queue is empty after dequeue
	size, err = testQueue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get size after dequeue: %v", err)
	}
	if size != 0 {
		t.Errorf("Expected queue size 0 after dequeue, got %d", size)
	}
}
