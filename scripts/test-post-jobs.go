package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	baseURL       = flag.String("url", "http://localhost:8080", "Base URL of the API server")
	skipRedis     = flag.Bool("skip-redis", false, "Skip Redis queue verification")
	passed        = 0
	failed        = 0
)

type TestResult struct {
	StatusCode int
	JobID      string
}

type CreateJobRequest struct {
	InputBucket  string `json:"input_bucket"`
	InputKey     string `json:"input_key"`
	OutputBucket string `json:"output_bucket"`
	OutputKey    string `json:"output_key,omitempty"`
	OutputPrefix string `json:"output_prefix,omitempty"`
	AttemptID    int    `json:"attempt_id,omitempty"`
}

type CreateJobResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func testCase(name string, testFunc func() (*TestResult, error), expectedStatus int) {
	fmt.Printf("测试: %s\n", name)
	
	result, err := testFunc()
	if err != nil {
		// Check if it's an HTTP error with the expected status code
		if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode == expectedStatus {
			fmt.Printf("  [OK] 通过 (预期错误: %d)\n", httpErr.StatusCode)
			passed++
			return
		}
		fmt.Printf("  [FAIL] 失败: %v\n", err)
		failed++
		return
	}
	
	if result != nil && result.StatusCode == expectedStatus {
		if result.JobID != "" {
			fmt.Printf("    Job ID: %s\n", result.JobID)
		}
		fmt.Printf("  [OK] 通过\n")
		passed++
	} else if result != nil {
		fmt.Printf("  [OK] 通过\n")
		passed++
	} else {
		fmt.Printf("  [FAIL] 失败: 状态码不匹配\n")
		failed++
	}
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

func makeRequest(method, url, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	
	return resp, nil
}

func test1_CreateJob() (*TestResult, error) {
	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		InputKey:     fmt.Sprintf("inputs/job-%s/data.zip", time.Now().Format("20060102150405")),
		OutputBucket: "test-bucket",
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	
	resp, err := makeRequest("POST", *baseURL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
	}
	
	var response CreateJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	
	return &TestResult{StatusCode: resp.StatusCode, JobID: response.JobID}, nil
}

func test2_RejectMultipart() (*TestResult, error) {
	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		InputKey:     "test.txt",
		OutputBucket: "test-bucket",
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	
	resp, err := makeRequest("POST", *baseURL+"/api/jobs", "multipart/form-data", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusUnsupportedMediaType {
		return &TestResult{StatusCode: resp.StatusCode}, nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	return nil, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
}

func test3_RejectLargeBody() (*TestResult, error) {
	// Create a body larger than 1MB
	largeData := strings.Repeat("x", int(1.5*1024*1024))
	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		InputKey:     "test.txt",
		OutputBucket: "test-bucket",
	}
	
	// Add large dummy data
	reqBodyMap := map[string]interface{}{
		"input_bucket":  reqBody.InputBucket,
		"input_key":     reqBody.InputKey,
		"output_bucket": reqBody.OutputBucket,
		"dummy_data":    largeData,
	}
	
	jsonData, err := json.Marshal(reqBodyMap)
	if err != nil {
		return nil, err
	}
	
	resp, err := makeRequest("POST", *baseURL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return &TestResult{StatusCode: resp.StatusCode}, nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	return nil, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
}

func test4_RejectNonJSON() (*TestResult, error) {
	bodyText := `{"input_bucket":"test","input_key":"test","output_bucket":"test"}`
	
	resp, err := makeRequest("POST", *baseURL+"/api/jobs", "text/plain", strings.NewReader(bodyText))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusUnsupportedMediaType {
		return &TestResult{StatusCode: resp.StatusCode}, nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	return nil, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
}

func test5_ValidateRequiredFields() (*TestResult, error) {
	reqBody := CreateJobRequest{
		InputBucket:  "test-bucket",
		OutputBucket: "test-bucket",
		// Missing input_key
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	
	resp, err := makeRequest("POST", *baseURL+"/api/jobs", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusBadRequest {
		return &TestResult{StatusCode: resp.StatusCode}, nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	return nil, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
}

func test6_RejectEmptyBody() (*TestResult, error) {
	resp, err := makeRequest("POST", *baseURL+"/api/jobs", "application/json", strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusBadRequest {
		return &TestResult{StatusCode: resp.StatusCode}, nil
	}
	
	body, _ := io.ReadAll(resp.Body)
	return nil, &HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
}

func checkRedisQueue() {
	if *skipRedis {
		return
	}
	
	fmt.Println()
	fmt.Println("验证 Redis 队列...")
	
	cmd := exec.Command("docker", "exec", "redis", "redis-cli", "LLEN", "jobs:pending")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  [WARN] Redis 不可用，跳过队列验证\n")
		return
	}
	
	queueSize := strings.TrimSpace(string(output))
	fmt.Printf("  [OK] Redis 队列大小: %s\n", queueSize)
}

func main() {
	flag.Parse()
	
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  POST /jobs API 测试")
	fmt.Println("========================================")
	fmt.Println()
	
	// Test 1: Create job successfully
	testCase("正常创建 Job", test1_CreateJob, http.StatusCreated)
	
	// Check Redis queue
	checkRedisQueue()
	
	// Test 2: Reject multipart/form-data
	testCase("拒绝 multipart/form-data", test2_RejectMultipart, http.StatusUnsupportedMediaType)
	
	// Test 3: Reject large body (>1MB)
	testCase("拒绝大文件 (>1MB)", test3_RejectLargeBody, http.StatusRequestEntityTooLarge)
	
	// Test 4: Reject non-JSON Content-Type
	testCase("拒绝非 JSON Content-Type", test4_RejectNonJSON, http.StatusUnsupportedMediaType)
	
	// Test 5: Validate required fields
	testCase("验证必需字段 (缺少 input_key)", test5_ValidateRequiredFields, http.StatusBadRequest)
	
	// Test 6: Reject empty body
	testCase("拒绝空 body", test6_RejectEmptyBody, http.StatusBadRequest)
	
	// Summary
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  测试总结")
	fmt.Println("========================================")
	fmt.Printf("通过: %d\n", passed)
	if failed == 0 {
		fmt.Printf("失败: %d\n", failed)
	} else {
		fmt.Printf("失败: %d\n", failed)
	}
	total := passed + failed
	fmt.Printf("总计: %d\n", total)
	fmt.Println()
	
	if failed == 0 {
		fmt.Println("[SUCCESS] 所有测试通过！")
		os.Exit(0)
	} else {
		fmt.Println("[FAILED] 部分测试失败")
		os.Exit(1)
	}
}
