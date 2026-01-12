# POST /jobs API 测试指南

本文档说明如何测试 Issue 4 实现的 POST /jobs 端点，包括安全护栏和 Redis 队列功能。

## 前置条件

### 1. 启动 Redis

#### 使用 Docker (推荐)
```powershell
docker run -d -p 6379:6379 --name redis redis:latest
```

#### 或使用本地 Redis
确保 Redis 服务运行在 `localhost:6379`

### 2. 配置环境变量

在 `cloud/` 目录创建 `.env` 文件：

```env
REDIS_URL=redis://localhost:6379/0
```

或使用独立配置：
```env
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_DATABASE=0
```

### 3. 启动服务器

```powershell
cd cloud
go run cmd/server/main.go -addr :8080 -dev
```

## 测试用例

### 测试 1: 正常创建 Job（成功场景）

**请求：**
```powershell
$body = @{
    input_bucket = "test-bucket"
    input_key = "inputs/job-123/data.zip"
    output_bucket = "test-bucket"
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
    -Method POST `
    -ContentType "application/json" `
    -Body $body
```

**预期响应：**
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "PENDING",
  "created_at": "2024-01-01T12:00:00Z"
}
```

**验证 Redis 队列：**
```powershell
# 使用 redis-cli 检查队列
docker exec -it redis redis-cli
> LLEN jobs:pending
> LRANGE jobs:pending 0 -1
```

应该能看到刚创建的 job_id 在队列中。

### 测试 2: 拒绝 multipart/form-data

**请求：**
```powershell
$formData = @{
    input_bucket = "test-bucket"
    input_key = "test.txt"
    output_bucket = "test-bucket"
}

Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
    -Method POST `
    -ContentType "multipart/form-data" `
    -Body $formData `
    -ErrorAction SilentlyContinue
```

**预期响应：**
- HTTP 415 Unsupported Media Type
- 错误信息：`"multipart/form-data is not allowed. Use OSS keys in JSON format only."`

### 测试 3: 拒绝大文件（>1MB）

**请求：**
```powershell
# 创建一个大于 1MB 的 JSON body
$largeBody = @{
    input_bucket = "test-bucket"
    input_key = "test.txt"
    output_bucket = "test-bucket"
    # 添加大量数据使其超过 1MB
    dummy_data = "x" * (2 * 1024 * 1024)  # 2MB
} | ConvertTo-Json -Depth 10

try {
    Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
        -Method POST `
        -ContentType "application/json" `
        -Body $largeBody `
        -ErrorAction Stop
} catch {
    Write-Host "Status: $($_.Exception.Response.StatusCode.value__)"
    Write-Host "Error: $($_.Exception.Message)"
}
```

**预期响应：**
- HTTP 413 Request Entity Too Large
- 错误信息包含：`"Request body exceeds maximum size of 1048576 bytes"`

### 测试 4: 拒绝非 JSON Content-Type

**请求：**
```powershell
$body = '{"input_bucket":"test","input_key":"test","output_bucket":"test"}'

Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
    -Method POST `
    -ContentType "text/plain" `
    -Body $body `
    -ErrorAction SilentlyContinue
```

**预期响应：**
- HTTP 415 Unsupported Media Type
- 错误信息：`"Content-Type must be application/json"`

### 测试 5: 验证 Redis 队列操作

**使用 curl (如果可用)：**
```powershell
# 创建多个 jobs
1..5 | ForEach-Object {
    $body = @{
        input_bucket = "test-bucket"
        input_key = "inputs/job-$_/data.zip"
        output_bucket = "test-bucket"
    } | ConvertTo-Json
    
    Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
        -Method POST `
        -ContentType "application/json" `
        -Body $body
}
```

**检查 Redis 队列：**
```powershell
docker exec redis redis-cli LLEN jobs:pending
# 应该返回 5

docker exec redis redis-cli LRANGE jobs:pending 0 -1
# 应该显示 5 个 job_id
```

### 测试 6: Redis 不可用时的降级

**步骤：**
1. 停止 Redis：`docker stop redis`
2. 创建 job（应该仍然成功）
3. 检查服务器日志，应该看到警告：`"Warning: Failed to connect to Redis"` 或 `"No queue configured"`

**请求：**
```powershell
$body = @{
    input_bucket = "test-bucket"
    input_key = "test.txt"
    output_bucket = "test-bucket"
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
    -Method POST `
    -ContentType "application/json" `
    -Body $body
```

**预期：**
- Job 仍然创建成功（返回 job_id）
- 服务器日志显示警告，但请求不失败
- Job 在数据库中，但不在 Redis 队列中

## 使用 curl 测试（跨平台）

### 成功创建 Job
```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "test-bucket",
    "input_key": "inputs/job-123/data.zip",
    "output_bucket": "test-bucket"
  }'
```

### 测试 multipart 拒绝
```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: multipart/form-data" \
  -F "input_bucket=test-bucket" \
  -F "input_key=test.txt" \
  -F "output_bucket=test-bucket" \
  -v
```

### 测试大文件拒绝
```bash
# 创建一个大于 1MB 的 JSON 文件
python -c "import json; data={'input_bucket':'test','input_key':'test','output_bucket':'test','dummy':'x'*2000000}; print(json.dumps(data))" > large.json

curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d @large.json \
  -v
```

## 自动化测试脚本

### Go 测试脚本（推荐）

使用 Go 编写的测试脚本 `scripts/test-post-jobs.go`：

```bash
# 运行所有测试
go run scripts/test-post-jobs.go

# 指定自定义 URL
go run scripts/test-post-jobs.go -url http://localhost:8080

# 跳过 Redis 检查
go run scripts/test-post-jobs.go -skip-redis
```

### PowerShell 测试脚本（可选）

创建一个 PowerShell 测试脚本 `test-post-jobs.ps1`：

```powershell
$baseUrl = "http://localhost:8080"

Write-Host "=== 测试 1: 正常创建 Job ===" -ForegroundColor Green
$body = @{
    input_bucket = "test-bucket"
    input_key = "inputs/test/data.zip"
    output_bucket = "test-bucket"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$baseUrl/api/jobs" `
        -Method POST `
        -ContentType "application/json" `
        -Body $body
    Write-Host "✓ Job 创建成功: $($response.job_id)" -ForegroundColor Green
    
    # 验证 Redis
    $queueSize = docker exec redis redis-cli LLEN jobs:pending
    Write-Host "✓ Redis 队列大小: $queueSize" -ForegroundColor Green
} catch {
    Write-Host "✗ 失败: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "`n=== 测试 2: 拒绝 multipart/form-data ===" -ForegroundColor Yellow
try {
    Invoke-RestMethod -Uri "$baseUrl/api/jobs" `
        -Method POST `
        -ContentType "multipart/form-data" `
        -Body $body `
        -ErrorAction Stop
    Write-Host "✗ 应该被拒绝但没有" -ForegroundColor Red
} catch {
    if ($_.Exception.Response.StatusCode.value__ -eq 415) {
        Write-Host "✓ 正确拒绝 multipart (415)" -ForegroundColor Green
    } else {
        Write-Host "✗ 错误的错误码: $($_.Exception.Response.StatusCode.value__)" -ForegroundColor Red
    }
}

Write-Host "`n=== 测试 3: 拒绝大文件 (>1MB) ===" -ForegroundColor Yellow
$largeBody = @{
    input_bucket = "test-bucket"
    input_key = "test.txt"
    output_bucket = "test-bucket"
    dummy_data = "x" * (2 * 1024 * 1024)
} | ConvertTo-Json -Depth 10

try {
    Invoke-RestMethod -Uri "$baseUrl/api/jobs" `
        -Method POST `
        -ContentType "application/json" `
        -Body $largeBody `
        -ErrorAction Stop
    Write-Host "✗ 应该被拒绝但没有" -ForegroundColor Red
} catch {
    if ($_.Exception.Response.StatusCode.value__ -eq 413) {
        Write-Host "✓ 正确拒绝大文件 (413)" -ForegroundColor Green
    } else {
        Write-Host "✗ 错误的错误码: $($_.Exception.Response.StatusCode.value__)" -ForegroundColor Red
    }
}

Write-Host "`n所有测试完成！" -ForegroundColor Cyan
```

运行测试：
```powershell
.\test-post-jobs.ps1
```

## 验证清单

- [ ] POST /jobs 成功返回 job_id
- [ ] Job 出现在 Redis 队列 `jobs:pending` 中
- [ ] multipart/form-data 请求返回 415
- [ ] 大于 1MB 的请求返回 413
- [ ] 非 JSON Content-Type 返回 415
- [ ] Redis 不可用时，job 仍能创建（降级处理）
- [ ] 服务器日志显示清晰的错误信息
