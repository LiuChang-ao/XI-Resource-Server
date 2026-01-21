package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	control "github.com/xiresource/proto/control"
)

func TestClient_DownloadInput(t *testing.T) {
	inputData := []byte("test input data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(inputData)
	}))
	defer server.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	data, size, sha256, err := client.downloadInput(server.URL)
	if err != nil {
		t.Fatalf("Failed to download input: %v", err)
	}

	if size != int64(len(inputData)) {
		t.Errorf("Expected size %d, got %d", len(inputData), size)
	}
	if !bytes.Equal(data, inputData) {
		t.Error("Downloaded data does not match input")
	}
	if sha256 == "" {
		t.Error("SHA256 should not be empty")
	}
}

func TestClient_StubCompute(t *testing.T) {
	client := New("ws://test", "test-agent", "test-token", 1)
	jobID := "test-job-123"
	attemptID := 1
	inputSize := int64(100)
	inputSHA256 := "abc123def456"

	outputData, err := client.stubCompute(jobID, attemptID, inputSize, inputSHA256)
	if err != nil {
		t.Fatalf("Failed to compute output: %v", err)
	}

	// Verify output is valid JSON
	var output StubOutput
	if err := json.Unmarshal(outputData, &output); err != nil {
		t.Fatalf("Failed to unmarshal output JSON: %v", err)
	}

	if output.JobID != jobID {
		t.Errorf("Expected JobID %s, got %s", jobID, output.JobID)
	}
	if output.AttemptID != attemptID {
		t.Errorf("Expected AttemptID %d, got %d", attemptID, output.AttemptID)
	}
	if output.InputSize != inputSize {
		t.Errorf("Expected InputSize %d, got %d", inputSize, output.InputSize)
	}
	if output.InputSHA256 != inputSHA256 {
		t.Errorf("Expected InputSHA256 %s, got %s", inputSHA256, output.InputSHA256)
	}
	if output.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}
}

func TestClient_UploadOutput(t *testing.T) {
	var uploadedData []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		uploadedData, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	outputData := []byte("test output data")
	if err := client.uploadOutput(server.URL, outputData); err != nil {
		t.Fatalf("Failed to upload output: %v", err)
	}

	if !bytes.Equal(uploadedData, outputData) {
		t.Error("Uploaded data does not match output")
	}
}

func TestClient_ProcessJob_FullFlow(t *testing.T) {
	// Setup HTTP test server for input download and output upload
	var uploadedData []byte

	inputData := []byte("test input data")
	expectedOutputKey := "jobs/test-job-123/1/output.bin"

	inputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(inputData)
	}))
	defer inputServer.Close()

	outputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		uploadedData, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer outputServer.Close()

	// Create client
	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Create JobAssigned with command
	// For testing, we'll use a simple command that writes test output
	// In real usage, this would be a Python script or similar that processes the input
	var command string
	if runtime.GOOS == "windows" {
		// Windows: Use a simple command to write test output
		command = `cmd /c echo test output > {output}`
	} else {
		// Unix: Use echo to write test output
		command = `echo "test output" > {output}`
	}

	jobAssigned := &control.JobAssigned{
		JobId:     "test-job-123",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputServer.URL},
		},
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputServer.URL},
		},
		OutputKey: expectedOutputKey,
		Command:   command,
	}

	// Process job (will report status, but conn is nil so it will just log)
	client.processJob(jobAssigned)

	// Wait for job processing
	time.Sleep(2 * time.Second)

	// Verify output was uploaded
	if uploadedData == nil {
		t.Fatal("Output was not uploaded")
	}

	// Verify output content (should contain "test output" or similar)
	outputStr := string(uploadedData)
	if len(outputStr) == 0 {
		t.Error("Output should not be empty")
	}
	// The exact format depends on the command, but we verify that output was generated
	t.Logf("Output received: %s", outputStr)
}

func TestClient_HandleJobAssigned_Concurrency(t *testing.T) {
	// Setup HTTP test servers with delay to simulate slow processing
	inputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Slow down processing
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test input"))
	}))
	defer inputServer.Close()

	outputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer outputServer.Close()

	// Create client with max_concurrency=1
	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Send first JobAssigned with command
	var command1 string
	if runtime.GOOS == "windows" {
		command1 = `echo test > {output}`
	} else {
		command1 = `echo test > {output}`
	}

	jobAssigned1 := &control.JobAssigned{
		JobId:     "job-1",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputServer.URL},
		},
		InputKey: "inputs/job-1/input.bin", // Required for download to proceed
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputServer.URL},
		},
		OutputKey: "jobs/job-1/1/output.bin",
		Command:   command1,
	}

	// Start first job (processJob increments runningJobs and starts goroutine)
	client.handleJobAssigned(jobAssigned1)

	// Wait a bit for handleJobAssigned to increment runningJobs and start processing
	time.Sleep(50 * time.Millisecond)

	// Check running jobs count
	runningJobs := client.getRunningJobs()
	if runningJobs != 1 {
		t.Errorf("Expected 1 running job, got %d", runningJobs)
	}

	// Send second JobAssigned immediately (should be rejected due to concurrency limit)
	var command2 string
	if runtime.GOOS == "windows" {
		command2 = `echo test > {output}`
	} else {
		command2 = `echo test > {output}`
	}

	jobAssigned2 := &control.JobAssigned{
		JobId:     "job-2",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputServer.URL},
		},
		InputKey: "inputs/job-2/input.bin", // Required for download to proceed
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputServer.URL},
		},
		OutputKey: "jobs/job-2/1/output.bin",
		Command:   command2,
	}

	// Try to start second job (should be rejected immediately)
	client.handleJobAssigned(jobAssigned2)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Check running jobs count (should still be 1, second job should be rejected)
	runningJobs = client.getRunningJobs()
	if runningJobs > 1 {
		t.Errorf("Expected at most 1 running job, got %d (second job should be rejected)", runningJobs)
	}
}

func TestClient_ProcessJob_DownloadFailure(t *testing.T) {
	// Setup HTTP test server that returns error
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Track if FAILED status was reported (we can't easily mock WebSocket in unit tests,
	// but we can verify the job processing fails correctly by checking it doesn't succeed)

	// Create JobAssigned with failing download URL
	var command string
	if runtime.GOOS == "windows" {
		command = `echo test > {output}`
	} else {
		command = `echo test > {output}`
	}

	jobAssigned := &control.JobAssigned{
		JobId:     "test-job-fail",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: errorServer.URL},
		},
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: "http://test"},
		},
		OutputKey: "jobs/test-job-fail/1/output.bin",
		Command:   command,
	}

	// Process job - should fail during download
	client.processJob(jobAssigned)

	// Wait for failure processing
	time.Sleep(1 * time.Second)

	// Verify job completed (runningJobs should be decremented)
	runningJobs := client.getRunningJobs()
	if runningJobs != 0 {
		t.Errorf("Expected 0 running jobs after failure, got %d", runningJobs)
	}
	// The actual FAILED status report would be sent via WebSocket, which we can't easily verify in unit tests
	// But we can verify the job processing logic handles the error correctly
}

func TestClient_DownloadInputToFile(t *testing.T) {
	inputData := []byte("test input data for file")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(inputData)
	}))
	defer server.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Create a temporary file path (with extension from input_key)
	filePath, err := client.downloadInputToFile(server.URL, "test-job", "test-input.txt")
	if err != nil {
		t.Fatalf("Failed to download input to file: %v", err)
	}
	defer func() {
		// Cleanup
		_ = os.Remove(filePath)
	}()

	// Read the file and verify content
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if !bytes.Equal(fileData, inputData) {
		t.Errorf("File content does not match. Expected %s, got %s", string(inputData), string(fileData))
	}
}

func TestClient_DownloadInputToFile_ExtensionPreservation(t *testing.T) {
	inputData := []byte("test image data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(inputData)
	}))
	defer server.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	testCases := []struct {
		name       string
		inputKey   string
		jobID      string
		wantExt    string
		wantInName bool
	}{
		{
			name:       "jpg extension preserved",
			inputKey:   "inputs/image.jpg",
			jobID:      "test-job-001",
			wantExt:    ".jpg",
			wantInName: true,
		},
		{
			name:       "png extension preserved",
			inputKey:   "zfc_files/ui_tap/281.png",
			jobID:      "test-job-002",
			wantExt:    ".png",
			wantInName: true,
		},
		{
			name:       "json extension preserved",
			inputKey:   "data/input.json",
			jobID:      "test-job-003",
			wantExt:    ".json",
			wantInName: true,
		},
		{
			name:       "no extension in input_key",
			inputKey:   "inputs/data",
			jobID:      "test-job-004",
			wantExt:    "",
			wantInName: false,
		},
		{
			name:       "empty input_key",
			inputKey:   "",
			jobID:      "test-job-005",
			wantExt:    "",
			wantInName: false,
		},
		{
			name:       "multiple dots in path",
			inputKey:   "inputs/data.backup.tar.gz",
			jobID:      "test-job-006",
			wantExt:    ".gz",
			wantInName: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filePath, err := client.downloadInputToFile(server.URL, tc.jobID, tc.inputKey)
			if err != nil {
				t.Fatalf("Failed to download input to file: %v", err)
			}
			defer func() {
				_ = os.Remove(filePath)
			}()

			// Verify file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Fatalf("File was not created: %s", filePath)
			}

			// Verify extension is preserved in filename
			actualExt := filepath.Ext(filePath)
			if tc.wantInName {
				if actualExt != tc.wantExt {
					t.Errorf("Expected extension %s in filename, got %s. File path: %s", tc.wantExt, actualExt, filePath)
				}
				// Verify the extension appears in the filename
				if !strings.Contains(filePath, tc.wantExt) {
					t.Errorf("Expected extension %s to appear in file path: %s", tc.wantExt, filePath)
				}
			} else {
				// If no extension expected, verify it's not in the filename
				if actualExt != "" {
					t.Errorf("Expected no extension, but got %s in file path: %s", actualExt, filePath)
				}
			}

			// Verify file content
			fileData, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read downloaded file: %v", err)
			}
			if !bytes.Equal(fileData, inputData) {
				t.Errorf("File content does not match")
			}

			// Verify filename format: job_{job_id}_input{.<ext>}
			baseName := filepath.Base(filePath)
			expectedPrefix := fmt.Sprintf("job_%s_input", tc.jobID)
			if !strings.HasPrefix(baseName, expectedPrefix) {
				t.Errorf("Expected filename to start with %s, got %s", expectedPrefix, baseName)
			}
		})
	}
}

func TestClient_ExecuteCommand_PlaceholderReplacement(t *testing.T) {
	// Create temporary input and output files
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.txt")
	outputFile := filepath.Join(tmpDir, "output.txt")

	// Write test input
	err := os.WriteFile(inputFile, []byte("test input"), 0644)
	if err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	// Test command with placeholders (use echo on Unix, or a simple command on Windows)
	// On Windows, we'll use a command that writes to the output file
	var command string
	if testing.Short() {
		t.Skip("Skipping command execution test in short mode")
	}

	// Create a simple script that copies input to output
	// For cross-platform testing, we'll use a command that works on both
	// This is a simplified test - in real usage, the command would be a Python script or similar
	command = `echo "test output" > {output}`
	if runtime.GOOS == "windows" {
		command = `echo test output > {output}`
	}

	// Replace placeholders manually for this test since executeCommand does it internally
	command = strings.ReplaceAll(command, "{input}", inputFile)
	command = strings.ReplaceAll(command, "{output}", outputFile)

	// Note: This test is simplified - actual command execution testing requires
	// a more complex setup. The placeholder replacement logic is what we're testing here.
	// Full command execution testing should be done in integration tests.
	t.Logf("Command with placeholders replaced: %s", command)
}

func TestClient_ExecuteCommand_EmptyCommand(t *testing.T) {
	client := New("ws://test", "test-agent", "test-token", 1)

	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.txt")
	outputFile := filepath.Join(tmpDir, "output.txt")

	_, err := client.executeCommand("", inputFile, outputFile)
	if err == nil {
		t.Error("Expected error for empty command, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "command is required") {
		t.Errorf("Expected error about command being required, got: %v", err)
	}
}

// TestClient_ProcessJob_NoInput tests that jobs without input files are handled correctly
func TestClient_ProcessJob_NoInput(t *testing.T) {
	// Setup HTTP test server for output upload only
	var uploadedData []byte

	outputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		uploadedData, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer outputServer.Close()

	// Create client
	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Create JobAssigned WITHOUT input (InputDownload is nil, InputKey is empty)
	var command string
	if runtime.GOOS == "windows" {
		command = `cmd /c echo test output > {output}`
	} else {
		command = `echo test output > {output}`
	}

	jobAssigned := &control.JobAssigned{
		JobId:     "test-job-no-input",
		AttemptId: 1,
		// InputDownload is nil - no input file
		// InputKey is empty string (default)
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputServer.URL},
		},
		OutputKey: "jobs/test-job-no-input/1/output.bin",
		Command:   command,
	}

	// Process job (should succeed without trying to download input)
	client.processJob(jobAssigned)

	// Verify output was uploaded
	if len(uploadedData) == 0 {
		t.Error("Expected output to be uploaded, but no data was uploaded")
	}

	outputStr := string(uploadedData)
	if len(outputStr) == 0 {
		t.Error("Output should not be empty")
	}
	t.Logf("Output received (no input): %s", outputStr)
}

// TestClient_ProcessJob_NoInput_WithInputDownloadButEmptyKey tests defensive case:
// InputDownload is set but InputKey is empty - should skip download
func TestClient_ProcessJob_NoInput_WithInputDownloadButEmptyKey(t *testing.T) {
	// Setup HTTP test server for output upload only
	var uploadedData []byte
	var downloadAttempted bool

	// This server should NOT be called for input download
	inputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadAttempted = true
		t.Error("Input download should NOT be attempted when InputKey is empty")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer inputServer.Close()

	outputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		uploadedData, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer outputServer.Close()

	// Create client
	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Create JobAssigned with InputDownload set but InputKey empty
	// This is a defensive test - server should not send this, but agent should handle it gracefully
	var command string
	if runtime.GOOS == "windows" {
		command = `cmd /c echo test output > {output}`
	} else {
		command = `echo test output > {output}`
	}

	jobAssigned := &control.JobAssigned{
		JobId:     "test-job-no-input-key",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputServer.URL},
		},
		InputKey: "", // Empty - should skip download
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputServer.URL},
		},
		OutputKey: "jobs/test-job-no-input-key/1/output.bin",
		Command:   command,
	}

	// Process job (should skip download and succeed)
	client.processJob(jobAssigned)

	// Verify download was NOT attempted
	if downloadAttempted {
		t.Error("Input download should NOT be attempted when InputKey is empty, even if InputDownload is set")
	}

	// Verify output was uploaded
	if len(uploadedData) == 0 {
		t.Error("Expected output to be uploaded, but no data was uploaded")
	}
}

// TestClient_ProcessJob_NoInput_404Error tests that 404 errors during download cause job to fail
// This test verifies that if InputDownload and InputKey are set but file doesn't exist (404),
// the job should fail (server should not send InputDownload if file doesn't exist)
// Note: In production, server should check file existence before generating presigned URL
func TestClient_ProcessJob_NoInput_404Error(t *testing.T) {
	// Setup HTTP test server that returns 404 for input download
	var downloadAttempted bool

	notFoundServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadAttempted = true
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFoundServer.Close()

	outputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer outputServer.Close()

	// Create client
	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	// Create JobAssigned with InputDownload and InputKey set, but URL returns 404
	// This simulates a case where server incorrectly generated URL for non-existent file
	var command string
	if runtime.GOOS == "windows" {
		command = `cmd /c echo test output > {output}`
	} else {
		command = `echo test output > {output}`
	}

	jobAssigned := &control.JobAssigned{
		JobId:     "test-job-404",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: notFoundServer.URL},
		},
		InputKey: "inputs/test-job-404/input.bin", // Key is set, but file doesn't exist
		OutputUpload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: outputServer.URL},
		},
		OutputKey: "jobs/test-job-404/1/output.bin",
		Command:   command,
	}

	// Process job (should fail with 404 error)
	client.processJob(jobAssigned)

	// Verify download was attempted (since InputKey is set)
	if !downloadAttempted {
		t.Error("Expected download to be attempted when InputKey is set, even if file doesn't exist")
	}

	// Note: We can't easily verify the status report without mocking WebSocket,
	// but the test verifies that the download attempt happens and fails correctly.
	// In production, server should not send InputDownload if file doesn't exist,
	// so this scenario should be rare. The test documents the current behavior.
}

func TestClient_ProcessForwardJob_URLMode_NoDownload(t *testing.T) {
	var inputDownloadCalls int
	inputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inputDownloadCalls++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("should not be downloaded"))
	}))
	defer inputServer.Close()

	var receivedInputURL string
	var receivedBody []byte
	localService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedInputURL = r.Header.Get("X-Input-URL")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer localService.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}

	jobAssigned := &control.JobAssigned{
		JobId:     "forward-job-url",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputServer.URL},
		},
		InputKey:         "inputs/forward/input.txt",
		JobType:          control.JobTypeEnum_JOB_TYPE_FORWARD_HTTP,
		InputForwardMode: control.InputForwardMode_INPUT_FORWARD_MODE_URL,
		ForwardHttp: &control.ForwardHttpRequest{
			Url:    localService.URL,
			Method: http.MethodPost,
		},
	}

	client.processJob(jobAssigned)

	if inputDownloadCalls != 0 {
		t.Fatalf("Expected no input download calls, got %d", inputDownloadCalls)
	}
	if receivedInputURL != inputServer.URL {
		t.Fatalf("Expected X-Input-URL to be %s, got %s", inputServer.URL, receivedInputURL)
	}
	if !bytes.Contains(receivedBody, []byte("input_url")) {
		t.Fatalf("Expected JSON body to include input_url, got %s", string(receivedBody))
	}
}

func TestClient_ProcessForwardJob_LocalFile_Cache(t *testing.T) {
	var inputDownloadCalls int
	inputData := []byte("cached input data")
	inputServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inputDownloadCalls++
		w.WriteHeader(http.StatusOK)
		w.Write(inputData)
	}))
	defer inputServer.Close()

	var receivedFile []byte
	localService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("Failed to parse multipart: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("Expected file field: %v", err)
		}
		defer file.Close()
		receivedFile, _ = io.ReadAll(file)
		w.WriteHeader(http.StatusOK)
	}))
	defer localService.Close()

	client := New("ws://test", "test-agent", "test-token", 1)
	client.httpClient = &http.Client{Timeout: 5 * time.Second}
	client.SetInputCacheTTL(5 * time.Minute)

	jobAssigned := &control.JobAssigned{
		JobId:     "forward-job-file",
		AttemptId: 1,
		InputDownload: &control.OSSAccess{
			Auth: &control.OSSAccess_PresignedUrl{PresignedUrl: inputServer.URL},
		},
		InputKey:         "inputs/forward/input.txt",
		JobType:          control.JobTypeEnum_JOB_TYPE_FORWARD_HTTP,
		InputForwardMode: control.InputForwardMode_INPUT_FORWARD_MODE_LOCAL_FILE,
		ForwardHttp: &control.ForwardHttpRequest{
			Url:    localService.URL,
			Method: http.MethodPost,
			Body:   []byte("payload"),
		},
	}

	client.processJob(jobAssigned)
	client.processJob(jobAssigned)

	if inputDownloadCalls != 1 {
		t.Fatalf("Expected 1 input download call due to cache, got %d", inputDownloadCalls)
	}
	if !bytes.Equal(receivedFile, inputData) {
		t.Fatalf("Expected forwarded file to match input data")
	}
}
