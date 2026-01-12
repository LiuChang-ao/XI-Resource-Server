# 使用 Go Test 运行测试

标准的 Go 测试文件位于 `cloud/internal/api/api_test.go`，可以使用标准的 `go test` 命令运行。

## 运行所有测试

**重要：必须在 `cloud` 目录下运行测试**

```powershell
cd cloud
go test ./internal/api/...
```

或者从项目根目录：

```powershell
cd cloud; go test ./internal/api/...
```

## 运行单个测试

```powershell
cd cloud
go test ./internal/api/... -run TestHandleCreateJob_Success
```

## 运行测试并显示详细输出

```powershell
cd cloud
go test ./internal/api/... -v
```

## 运行测试并显示覆盖率

```powershell
cd cloud
go test ./internal/api/... -cover
```

## 运行测试并生成覆盖率报告

```powershell
cd cloud
go test ./internal/api/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 跳过集成测试（快速测试）

某些测试需要 Redis，可以使用 `-short` 标志跳过：

```powershell
cd cloud
go test ./internal/api/... -short
```

## 运行所有 cloud 模块的测试

```powershell
cd cloud
go test ./...
```

## 测试列表

测试文件包含以下测试用例：

1. **TestHandleCreateJob_Success** - 正常创建 Job（返回 201）
2. **TestHandleCreateJob_RejectMultipart** - 拒绝 multipart/form-data（返回 415）
3. **TestHandleCreateJob_RejectLargeBody** - 拒绝大于 1MB 的请求（返回 413）
4. **TestHandleCreateJob_RejectNonJSON** - 拒绝非 JSON Content-Type（返回 415）
5. **TestHandleCreateJob_ValidateRequiredFields** - 验证必需字段（返回 400）
6. **TestHandleCreateJob_RejectEmptyBody** - 拒绝空 body（返回 400）
7. **TestHandleCreateJob_RedisEnqueue** - 验证 Redis 队列入队（需要 Redis）
8. **TestRedisQueueIntegration** - Redis 队列集成测试（需要 Redis 容器）

## 前置条件

### 对于需要 Redis 的测试

如果运行包含 Redis 的测试，需要：

1. 启动 Redis：
   ```powershell
   docker run -d -p 6379:6379 --name redis redis:latest
   ```

2. 配置环境变量（可选，使用默认 localhost:6379）：
   ```env
   REDIS_URL=redis://localhost:6379/0
   ```

如果 Redis 不可用，相关测试会自动跳过（使用 `t.Skip`）。

## 与 CI/CD 集成

在 CI/CD 流水线中：

```yaml
# GitHub Actions 示例
- name: Run tests
  run: |
    cd cloud
    go test ./internal/api/... -v -coverprofile=coverage.out

- name: Run tests with Redis
  run: |
    docker run -d -p 6379:6379 --name redis redis:latest
    cd cloud
    go test ./internal/api/... -v
```
