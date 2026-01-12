# 快速测试指南

## 1. 启动 Redis

```powershell
# 使用 docker-compose (推荐)
cd infra
docker-compose up -d redis

# 或直接使用 docker
docker run -d -p 6379:6379 --name redis redis:latest
```

## 2. 配置环境变量

在 `cloud/` 目录创建 `.env` 文件：

```env
REDIS_URL=redis://localhost:6379/0
```

## 3. 启动服务器

```powershell
cd cloud
go run cmd/server/main.go -addr :8080 -dev
```

## 4. 运行测试

### 使用 Go Test（推荐）

```powershell
# 进入 cloud 目录
cd cloud

# 运行所有 API 测试
go test ./internal/api/... -v

# 运行单个测试
go test ./internal/api/... -run TestHandleCreateJob_Success -v

# 跳过需要 Redis 的测试
go test ./internal/api/... -short
```

### 使用 Go 测试脚本（可选）

```powershell
# 在项目根目录
go run scripts/test-post-jobs.go

# 或指定自定义 URL
go run scripts/test-post-jobs.go -url http://localhost:8080

# 跳过 Redis 检查
go run scripts/test-post-jobs.go -skip-redis
```

### 使用 PowerShell 测试（可选）

```powershell
# 在项目根目录
.\scripts\test-post-jobs.ps1
```

## 5. 手动测试示例

### 成功创建 Job
```powershell
$body = @{
    input_bucket = "test-bucket"
    input_key = "inputs/test/data.zip"
    output_bucket = "test-bucket"
} | ConvertTo-Json

Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" `
    -Method POST `
    -ContentType "application/json" `
    -Body $body
```

### 检查 Redis 队列
```powershell
docker exec redis redis-cli LLEN jobs:pending
docker exec redis redis-cli LRANGE jobs:pending 0 -1
```

### 测试拒绝 multipart
```powershell
curl -X POST http://localhost:8080/api/jobs `
  -H "Content-Type: multipart/form-data" `
  -F "input_bucket=test" `
  -F "input_key=test" `
  -F "output_bucket=test"
```

预期：HTTP 415 错误
