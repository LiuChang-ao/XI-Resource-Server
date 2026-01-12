package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/tencentyun/cos-go-sdk-v5"
)

const (
	serverPort = "8081" // Use different port to avoid conflicts with M0_e2e.go
	serverURL  = "http://localhost:" + serverPort
	wssURL     = "ws://localhost:" + serverPort + "/wss"
)

// Environment variables for OSS e2e test
const (
	envOSSEndpoint  = "E2E_OSS_ENDPOINT" // Optional, uses COS_BASE_URL if not set
	envOSSBucket    = "E2E_OSS_BUCKET"   // Uses COS_BUCKET if not set
	envOSSRegion    = "E2E_OSS_REGION"   // Uses COS_REGION if not set
	envOSSAccessKey = "E2E_OSS_ACCESS_KEY_ID"
	envOSSSecretKey = "E2E_OSS_ACCESS_KEY_SECRET"
	envOSSPrefix    = "E2E_OSS_PREFIX" // Default: "e2e/test/"
)

// Environment variables for MySQL e2e test
const (
	envMySQLHost     = "E2E_MYSQL_HOST"
	envMySQLPort     = "E2E_MYSQL_PORT"
	envMySQLUser     = "E2E_MYSQL_USER"
	envMySQLPassword = "E2E_MYSQL_PASSWORD"
	envMySQLDatabase = "E2E_MYSQL_DATABASE"
)

type CreateJobRequest struct {
	InputBucket  string `json:"input_bucket"`
	InputKey     string `json:"input_key"`
	OutputBucket string `json:"output_bucket"`
}

type CreateJobResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	fmt.Println("=== E2E Test (OSS): Create Job → Agent Pull → Download Input → Upload Output → Job SUCCEEDED ===")
	testStart := time.Now()

	// Check if OSS credentials are configured
	ossConfig, err := loadOSSConfig()
	if err != nil {
		fmt.Printf("⚠ Skipping OSS e2e test: %v\n", err)
		fmt.Println("To run this test, set the following environment variables:")
		fmt.Println("  E2E_OSS_ACCESS_KEY_ID (or COS_SECRET_ID)")
		fmt.Println("  E2E_OSS_ACCESS_KEY_SECRET (or COS_SECRET_KEY)")
		fmt.Println("  E2E_OSS_BUCKET (or COS_BUCKET)")
		fmt.Println("  E2E_OSS_REGION (or COS_REGION)")
		fmt.Println("Optional: E2E_OSS_PREFIX (default: e2e/test/)")
		os.Exit(0) // Exit with 0 to indicate skip, not failure
	}

	fmt.Printf("✓ OSS configuration loaded (bucket: %s, region: %s, prefix: %s)\n",
		ossConfig.Bucket, ossConfig.Region, getOSSPrefix())

	// Check MySQL configuration
	mysqlConfig := loadMySQLConfig()
	if mysqlConfig != nil {
		fmt.Printf("✓ MySQL configuration loaded (host: %s, port: %d, database: %s)\n",
			mysqlConfig.Host, mysqlConfig.Port, mysqlConfig.Database)
	} else {
		fmt.Println("⚠ MySQL not configured, server will use SQLite (default)")
		fmt.Println("To use MySQL, set the following environment variables:")
		fmt.Println("  E2E_MYSQL_HOST (or MYSQL_HOST)")
		fmt.Println("  E2E_MYSQL_PORT (or MYSQL_PORT, default: 3306)")
		fmt.Println("  E2E_MYSQL_USER (or MYSQL_USER)")
		fmt.Println("  E2E_MYSQL_PASSWORD (or MYSQL_PASSWORD)")
		fmt.Println("  E2E_MYSQL_DATABASE (or MYSQL_DATABASE)")
	}

	projectRoot := ".."

	// Try to use pre-built binaries first
	fmt.Println("\n[0/6] Locating binaries...")
	serverBin, agentBin, needCleanup, err := findOrBuildBinaries(projectRoot)
	if err != nil {
		fmt.Printf("✗ Failed to locate or build binaries: %v\n", err)
		os.Exit(1)
	}
	if needCleanup {
		defer func() {
			fmt.Println("\nCleaning up temporary binaries...")
			cleanupBinaries(serverBin, agentBin)
		}()
	} else {
		fmt.Println("Using pre-built binaries (will not cleanup)")
	}

	// Create OSS provider for test operations
	// We'll use the COS SDK directly in the test since we can't easily import cloud/internal/oss
	// This is acceptable for e2e tests
	testProvider, err := newCOSTestProvider(ossConfig)
	if err != nil {
		fmt.Printf("✗ Failed to create OSS provider: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: Upload input object to OSS
	fmt.Println("\n[1/6] Uploading input object to OSS...")
	timestamp := time.Now().UnixNano()
	inputKey := fmt.Sprintf("%sinputs/%d/input.bin", getOSSPrefix(), timestamp)
	inputData := []byte(fmt.Sprintf("test-input-data-%d", timestamp))
	if err := testProvider.PutObject(ctx, inputKey, inputData); err != nil {
		fmt.Printf("✗ Failed to upload input object: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Uploaded input object: %s\n", inputKey)

	// Cleanup function for input/output objects
	cleanupObjects := func() {
		fmt.Println("\nCleaning up test objects from OSS...")
		if err := testProvider.DeleteObject(ctx, inputKey); err != nil {
			fmt.Printf("⚠ Warning: Failed to delete input object %s: %v\n", inputKey, err)
		} else {
			fmt.Printf("✓ Deleted input object: %s\n", inputKey)
		}
	}
	defer cleanupObjects()

	// Step 2: Start cloud server
	fmt.Println("\n[2/6] Starting cloud server...")
	// Verify server binary exists and is readable
	absServerBin, err := filepath.Abs(serverBin)
	if err != nil {
		fmt.Printf("✗ Failed to get absolute path for server binary: %v\n", err)
		os.Exit(1)
	}

	if info, err := os.Stat(absServerBin); err != nil {
		fmt.Printf("✗ Server binary not found: %v\n", err)
		fmt.Printf("  Expected path: %s\n", absServerBin)
		os.Exit(1)
	} else {
		fmt.Printf("Server binary: %s (%d bytes)\n", absServerBin, info.Size())
	}

	// Use absolute path for execution
	serverCmd := exec.CommandContext(ctx, absServerBin, "-addr", ":"+serverPort, "-dev")
	serverCmd.Stdout = os.Stdout
	serverCmd.Stderr = os.Stderr
	
	// Set MySQL environment variables for server process if configured
	// This allows the server to use MySQL instead of SQLite
	// Reuse mysqlConfig loaded earlier in main()
	if mysqlConfig != nil {
		fmt.Println("Configuring server to use MySQL database...")
		serverCmd.Env = os.Environ()
		serverCmd.Env = append(serverCmd.Env, "DB_TYPE=mysql")
		serverCmd.Env = append(serverCmd.Env, fmt.Sprintf("MYSQL_HOST=%s", mysqlConfig.Host))
		serverCmd.Env = append(serverCmd.Env, fmt.Sprintf("MYSQL_PORT=%d", mysqlConfig.Port))
		serverCmd.Env = append(serverCmd.Env, fmt.Sprintf("MYSQL_USER=%s", mysqlConfig.User))
		serverCmd.Env = append(serverCmd.Env, fmt.Sprintf("MYSQL_PASSWORD=%s", mysqlConfig.Password))
		serverCmd.Env = append(serverCmd.Env, fmt.Sprintf("MYSQL_DATABASE=%s", mysqlConfig.Database))
		if mysqlConfig.Params != "" {
			serverCmd.Env = append(serverCmd.Env, fmt.Sprintf("MYSQL_PARAMS=%s", mysqlConfig.Params))
		}
		fmt.Printf("  MySQL config: %s@%s:%d/%s\n", mysqlConfig.User, mysqlConfig.Host, mysqlConfig.Port, mysqlConfig.Database)
	} else {
		fmt.Println("Server will use SQLite (MySQL not configured)")
	}
	
	if err := serverCmd.Start(); err != nil {
		fmt.Printf("✗ Failed to start server: %v\n", err)
		fmt.Printf("  Server binary path: %s\n", absServerBin)
		os.Exit(1)
	}
	defer terminateProcess(serverCmd, "server")

	// Wait for server to be ready
	fmt.Println("Waiting for server to be ready...")
	if !waitForServer(15 * time.Second) {
		fmt.Println("✗ Server failed to become ready (/health != 200)")
		os.Exit(1)
	}
	fmt.Println("✓ Server is ready")

	// Step 3: Start agent
	fmt.Println("\n[3/6] Starting agent...")
	agentID := fmt.Sprintf("test-agent-oss-%d", timestamp)
	// Use absolute path for agent binary
	absAgentBin, err := filepath.Abs(agentBin)
	if err != nil {
		fmt.Printf("✗ Failed to get absolute path for agent binary: %v\n", err)
		os.Exit(1)
	}

	agentCmd := exec.CommandContext(ctx, absAgentBin,
		"-server", wssURL,
		"-agent-id", agentID,
		"-agent-token", "dev-token",
		"-max-concurrency", "1",
	)
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	if err := agentCmd.Start(); err != nil {
		fmt.Printf("✗ Failed to start agent: %v\n", err)
		fmt.Printf("  Agent binary path: %s\n", absAgentBin)
		os.Exit(1)
	}
	defer terminateProcess(agentCmd, "agent")

	// Wait for agent to be online
	fmt.Println("Waiting for agent to register and become ONLINE...")
	if !waitForAgentOnline(agentID, 30*time.Second) {
		fmt.Printf("✗ Agent %s did not become ONLINE\n", agentID)
		os.Exit(1)
	}
	fmt.Printf("✓ Agent ONLINE: %s\n", agentID)

	// Step 4: Create job via API
	fmt.Println("\n[4/6] Creating job via API...")
	createReq := CreateJobRequest{
		InputBucket:  ossConfig.Bucket,
		InputKey:     inputKey,
		OutputBucket: ossConfig.Bucket,
	}
	jobID, err := createJob(createReq)
	if err != nil {
		fmt.Printf("✗ Failed to create job: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Job created: %s\n", jobID)

	// Step 5: Wait for job to complete
	fmt.Println("\n[5/6] Waiting for job to complete...")
	finalJob, err := waitForJobCompletion(jobID, 5*time.Minute)
	if err != nil {
		fmt.Printf("✗ Job did not complete: %v\n", err)
		os.Exit(1)
	}

	// Verify job status
	if finalJob.Status != "SUCCEEDED" {
		fmt.Printf("✗ Job status is %s, expected SUCCEEDED\n", finalJob.Status)
		os.Exit(1)
	}
	fmt.Printf("✓ Job completed with status: %s\n", finalJob.Status)

	// Step 6: Verify output object exists in OSS
	fmt.Println("\n[6/6] Verifying output object exists in OSS...")
	outputKey := finalJob.OutputKey
	if outputKey == "" {
		// If OutputKey is empty, try to construct from OutputPrefix
		if finalJob.OutputPrefix != "" {
			// Agent typically uploads to a file under the prefix
			// Try common output filenames
			possibleKeys := []string{
				finalJob.OutputPrefix + "output.json",
				finalJob.OutputPrefix + "result.json",
				finalJob.OutputPrefix + "output",
			}
			found := false
			for _, key := range possibleKeys {
				exists, err := testProvider.ObjectExists(ctx, key)
				if err == nil && exists {
					outputKey = key
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("✗ Could not find output object under prefix %s\n", finalJob.OutputPrefix)
				os.Exit(1)
			}
		} else {
			fmt.Printf("✗ Job has no OutputKey or OutputPrefix\n")
			os.Exit(1)
		}
	}

	exists, err := testProvider.ObjectExists(ctx, outputKey)
	if err != nil {
		fmt.Printf("✗ Failed to check output object existence: %v\n", err)
		os.Exit(1)
	}
	if !exists {
		fmt.Printf("✗ Output object does not exist: %s\n", outputKey)
		os.Exit(1)
	}
	fmt.Printf("✓ Output object exists: %s\n", outputKey)

	// Optional: Download and verify output content
	fmt.Println("Downloading output object to verify content...")
	downloadURL, err := testProvider.GenerateDownloadURL(ctx, outputKey)
	if err != nil {
		fmt.Printf("⚠ Warning: Failed to generate download URL: %v\n", err)
	} else {
		resp, err := http.Get(downloadURL)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				data, err := io.ReadAll(resp.Body)
				if err == nil {
					// Verify output contains job_id
					if strings.Contains(string(data), jobID) {
						fmt.Printf("✓ Output content verified (contains job_id)\n")
					} else {
						fmt.Printf("⚠ Warning: Output content does not contain job_id\n")
					}
				}
			}
		}
	}

	// Update cleanup to include output object
	originalCleanup := cleanupObjects
	cleanupObjects = func() {
		originalCleanup()
		if outputKey != "" {
			if err := testProvider.DeleteObject(ctx, outputKey); err != nil {
				fmt.Printf("⚠ Warning: Failed to delete output object %s: %v\n", outputKey, err)
			} else {
				fmt.Printf("✓ Deleted output object: %s\n", outputKey)
			}
		}
	}

	fmt.Println("\n=== E2E Test (OSS) PASSED ===")
	fmt.Printf("Test duration: %v\n", time.Since(testStart))
}

type OSSConfig struct {
	SecretID   string
	SecretKey  string
	Bucket     string
	Region     string
	BaseURL    string
	PresignTTL time.Duration
}

func loadOSSConfig() (OSSConfig, error) {
	// Try to load .env file (ignore error if file doesn't exist)
	// Try multiple locations: scripts/, cloud/, project root, and parent directories
	envPaths := []string{
		".env",          // scripts/.env
		"../cloud/.env", // cloud/.env (most common location)
		"../.env",       // project root .env
		"../../.env",    // parent of project root
		"../../../.env", // 3 levels up
	}

	for _, envPath := range envPaths {
		// Try to load .env file (godotenv.Load returns error if file doesn't exist)
		if err := godotenv.Load(envPath); err == nil {
			break // Successfully loaded .env file
		}
	}

	// Try to load from E2E-specific env vars first, then fall back to COS_* vars
	accessKeyID := os.Getenv(envOSSAccessKey)
	if accessKeyID == "" {
		accessKeyID = os.Getenv("COS_SECRET_ID")
	}

	secretKey := os.Getenv(envOSSSecretKey)
	if secretKey == "" {
		secretKey = os.Getenv("COS_SECRET_KEY")
	}

	bucket := os.Getenv(envOSSBucket)
	if bucket == "" {
		bucket = os.Getenv("COS_BUCKET")
	}

	region := os.Getenv(envOSSRegion)
	if region == "" {
		region = os.Getenv("COS_REGION")
	}

	if accessKeyID == "" || secretKey == "" || bucket == "" || region == "" {
		return OSSConfig{}, fmt.Errorf("OSS credentials not configured")
	}

	config := OSSConfig{
		SecretID:  accessKeyID,
		SecretKey: secretKey,
		Bucket:    bucket,
		Region:    region,
	}

	// Use E2E_OSS_ENDPOINT or COS_BASE_URL if set
	baseURL := os.Getenv(envOSSEndpoint)
	if baseURL == "" {
		baseURL = os.Getenv("COS_BASE_URL")
	}
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	// Load TTL from COS_PRESIGN_TTL_MINUTES if set
	if ttlStr := os.Getenv("COS_PRESIGN_TTL_MINUTES"); ttlStr != "" {
		var ttlMinutes int
		if _, err := fmt.Sscanf(ttlStr, "%d", &ttlMinutes); err == nil && ttlMinutes > 0 {
			config.PresignTTL = time.Duration(ttlMinutes) * time.Minute
		}
	} else {
		config.PresignTTL = 15 * time.Minute
	}

	return config, nil
}

// COSTestProvider wraps COS SDK for e2e testing
type COSTestProvider interface {
	ObjectExists(ctx context.Context, key string) (bool, error)
	PutObject(ctx context.Context, key string, data []byte) error
	DeleteObject(ctx context.Context, key string) error
	GenerateDownloadURL(ctx context.Context, key string) (string, error)
}

type cosTestProvider struct {
	client     *cos.Client
	config     OSSConfig
	secretID   string
	secretKey  string
	presignTTL time.Duration
}

func newCOSTestProvider(config OSSConfig) (COSTestProvider, error) {
	// Import cos SDK here to avoid import issues
	// We'll need to add the import at the top
	baseURLStr := config.BaseURL
	if baseURLStr == "" {
		baseURLStr = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", config.Bucket, config.Region)
	}
	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	b := &cos.BaseURL{BucketURL: baseURL}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  config.SecretID,
			SecretKey: config.SecretKey,
		},
	})

	presignTTL := config.PresignTTL
	if presignTTL == 0 {
		presignTTL = 15 * time.Minute
	}

	return &cosTestProvider{
		client:     client,
		config:     config,
		secretID:   config.SecretID,
		secretKey:  config.SecretKey,
		presignTTL: presignTTL,
	}, nil
}

func (p *cosTestProvider) ObjectExists(ctx context.Context, key string) (bool, error) {
	exists, err := p.client.Object.IsExist(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}
	return exists, nil
}

func (p *cosTestProvider) PutObject(ctx context.Context, key string, data []byte) error {
	_, err := p.client.Object.Put(ctx, key, bytes.NewReader(data), nil)
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	return nil
}

func (p *cosTestProvider) DeleteObject(ctx context.Context, key string) error {
	_, err := p.client.Object.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func (p *cosTestProvider) GenerateDownloadURL(ctx context.Context, key string) (string, error) {
	presignedURL, err := p.client.Object.GetPresignedURL(
		ctx,
		http.MethodGet,
		key,
		p.secretID,
		p.secretKey,
		p.presignTTL,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate download URL: %w", err)
	}
	return presignedURL.String(), nil
}

func getOSSPrefix() string {
	prefix := os.Getenv(envOSSPrefix)
	if prefix == "" {
		prefix = "e2e/test/"
	}
	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func waitForServer(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serverURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func waitForAgentOnline(agentID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serverURL + "/api/agents/online")
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var agents []struct {
					AgentID string `json:"agent_id"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&agents); err == nil {
					for _, a := range agents {
						if a.AgentID == agentID {
							return true
						}
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func createJob(req CreateJobRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(serverURL+"/api/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var createResp CreateJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return createResp.JobID, nil
}

type Job struct {
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	OutputKey    string `json:"output_key"`
	OutputPrefix string `json:"output_prefix"`
}

func isTerminalStatus(status string) bool {
	return status == "SUCCEEDED" || status == "FAILED" || status == "CANCELED" || status == "LOST"
}

func waitForJobCompletion(jobID string, timeout time.Duration) (*Job, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serverURL + "/api/jobs/" + jobID)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var j Job
			if err := json.NewDecoder(resp.Body).Decode(&j); err == nil {
				if isTerminalStatus(j.Status) {
					return &j, nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timeout waiting for job %s to complete", jobID)
}

func terminateProcess(cmd *exec.Cmd, name string) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	fmt.Printf("\nTerminating %s (PID: %d)...\n", name, pid)

	if err := cmd.Process.Kill(); err != nil {
		fmt.Printf("Warning: failed to kill %s (PID: %d): %v\n", name, pid, err)
		return
	}

	done := make(chan error, 1)
	go func() {
		_, err := cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-done:
		fmt.Printf("%s (PID: %d) terminated successfully\n", name, pid)
	case <-time.After(5 * time.Second):
		fmt.Printf("Warning: %s (PID: %d) did not exit within 5 seconds\n", name, pid)
	}
}

// findOrBuildBinaries tries to find pre-built binaries first, then builds if needed
// Returns: serverBin, agentBin, needCleanup (true if built, false if pre-built), error
func findOrBuildBinaries(projectRoot string) (serverBin, agentBin string, needCleanup bool, err error) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	// Try pre-built binaries in bin/ directory first
	prebuiltServer := filepath.Join(projectRoot, "bin", "server"+ext)
	prebuiltAgent := filepath.Join(projectRoot, "bin", "agent"+ext)

	if fileExists(prebuiltServer) && fileExists(prebuiltAgent) {
		absServer, err1 := filepath.Abs(prebuiltServer)
		absAgent, err2 := filepath.Abs(prebuiltAgent)
		if err1 == nil && err2 == nil {
			fmt.Printf("✓ Using pre-built server: %s\n", absServer)
			fmt.Printf("✓ Using pre-built agent: %s\n", absAgent)
			return absServer, absAgent, false, nil
		}
	}

	// Pre-built binaries not found, build them
	fmt.Println("Pre-built binaries not found in bin/, building binaries...")
	serverBin, agentBin, err = buildBinaries(projectRoot)
	if err != nil {
		return "", "", false, err
	}
	return serverBin, agentBin, true, nil
}

func buildBinaries(projectRoot string) (serverBin, agentBin string, err error) {
	tmpDir := os.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	serverBin = filepath.Join(tmpDir, fmt.Sprintf("e2e-oss-server-%d%s", time.Now().UnixNano(), ext))
	agentBin = filepath.Join(tmpDir, fmt.Sprintf("e2e-oss-agent-%d%s", time.Now().UnixNano(), ext))

	fmt.Printf("Building server to %s...\n", serverBin)
	buildServer := exec.Command("go", "build", "-o", serverBin, "./cmd/server")
	buildServer.Dir = filepath.Join(projectRoot, "cloud")
	buildServer.Stdout = os.Stdout
	buildServer.Stderr = os.Stderr
	if err := buildServer.Run(); err != nil {
		return "", "", fmt.Errorf("failed to build server: %w", err)
	}

	fmt.Printf("Building agent to %s...\n", agentBin)
	buildAgent := exec.Command("go", "build", "-o", agentBin, "./cmd/agent")
	buildAgent.Dir = filepath.Join(projectRoot, "agent")
	buildAgent.Stdout = os.Stdout
	buildAgent.Stderr = os.Stderr
	if err := buildAgent.Run(); err != nil {
		return "", "", fmt.Errorf("failed to build agent: %w", err)
	}

	return serverBin, agentBin, nil
}

func cleanupBinaries(serverBin, agentBin string) {
	if serverBin != "" {
		os.Remove(serverBin)
	}
	if agentBin != "" {
		os.Remove(agentBin)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MySQLConfig holds MySQL connection configuration for e2e tests
type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	Params   string
}

// loadMySQLConfig loads MySQL configuration from environment variables
// Returns nil if MySQL is not configured (will fall back to SQLite)
func loadMySQLConfig() *MySQLConfig {
	// Try to load .env file first (same paths as OSS config)
	envPaths := []string{
		".env",          // scripts/.env
		"../cloud/.env", // cloud/.env (most common location)
		"../.env",       // project root .env
		"../../.env",    // parent of project root
		"../../../.env", // 3 levels up
	}

	for _, envPath := range envPaths {
		if err := godotenv.Load(envPath); err == nil {
			break // Successfully loaded .env file
		}
	}

	// Try E2E-specific MySQL env vars first, then fall back to standard MySQL env vars
	host := os.Getenv(envMySQLHost)
	if host == "" {
		host = os.Getenv("MYSQL_HOST")
	}

	portStr := os.Getenv(envMySQLPort)
	if portStr == "" {
		portStr = os.Getenv("MYSQL_PORT")
	}
	port := 3306 // default
	if portStr != "" {
		if p, err := fmt.Sscanf(portStr, "%d", &port); err != nil || p != 1 {
			port = 3306 // fallback to default
		}
	}

	user := os.Getenv(envMySQLUser)
	if user == "" {
		user = os.Getenv("MYSQL_USER")
	}

	password := os.Getenv(envMySQLPassword)
	if password == "" {
		password = os.Getenv("MYSQL_PASSWORD")
	}

	database := os.Getenv(envMySQLDatabase)
	if database == "" {
		database = os.Getenv("MYSQL_DATABASE")
	}

	params := os.Getenv("MYSQL_PARAMS")
	if params == "" {
		params = "charset=utf8mb4&parseTime=True&loc=Local"
	}

	// If all required fields are set, return config
	if host != "" && user != "" && password != "" && database != "" {
		return &MySQLConfig{
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Database: database,
			Params:   params,
		}
	}

	// MySQL not configured, will use SQLite
	return nil
}
