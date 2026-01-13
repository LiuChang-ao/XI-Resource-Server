# 测试指南

本文档说明如何运行和编写测试，包括单元测试、集成测试和端到端测试。

## 测试类型

### 1. 单元测试 (Unit Tests)

单元测试测试各个组件的独立功能。

#### 运行单元测试

**Windows (PowerShell):**
```powershell
# 运行所有单元测试
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 test

# 运行特定包的测试
cd cloud
go test ./...

cd agent
go test ./...

# 运行特定测试文件
go test ./internal/client/client_test.go ./internal/client/client.go

# 运行特定测试函数
go test -run TestClient_DownloadInputToFile_ExtensionPreservation ./internal/client
```

**Linux/macOS:**
```bash
# 运行所有单元测试
make test

# 运行特定包的测试
cd cloud && go test ./...
cd agent && go test ./...

# 运行特定测试函数
go test -run TestClient_DownloadInputToFile_ExtensionPreservation ./agent/internal/client
```

#### 测试覆盖范围

**Agent 测试 (`agent/internal/client/`):**
- `TestClient_DownloadInput`: 测试输入文件下载
- `TestClient_DownloadInputToFile`: 测试下载到临时文件
- `TestClient_DownloadInputToFile_ExtensionPreservation`: **测试文件扩展名保留功能**
- `TestClient_UploadOutput`: 测试输出文件上传
- `TestClient_ProcessJob_FullFlow`: 测试完整作业处理流程
- `TestClient_ExecuteCommand_PlaceholderReplacement`: 测试命令占位符替换
- `TestClient_HandleJobAssigned_Concurrency`: 测试并发控制

**Cloud Gateway 测试 (`cloud/internal/gateway/`):**
- `TestGateway_HandleRegister`: 测试Agent注册
- `TestGateway_HandleRequestJob`: 测试作业分配
- `TestGateway_HandleJobStatus`: 测试作业状态更新
- 各种错误处理和边界情况测试

**Cloud API 测试 (`cloud/internal/api/`):**
- `TestHandler_HandleCreateJob`: 测试作业创建API
- `TestHandler_HandleGetJob`: 测试作业查询API
- 请求验证和错误处理测试

**Job Store 测试 (`cloud/internal/job/`):**
- `TestStore_Create`: 测试作业创建
- `TestStore_UpdateStatus`: 测试状态更新
- `TestStore_UpdateAssignment`: 测试作业分配

### 2. 集成测试 (Integration Tests)

集成测试验证多个组件协同工作。

#### E2E 测试 (端到端测试)

**Windows (PowerShell):**
```powershell
# 运行基础E2E测试 (M0)
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 e2e

# 运行带真实OSS的E2E测试
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 e2e-oss
```

**Linux/macOS:**
```bash
# 运行基础E2E测试
make e2e

# 运行带真实OSS的E2E测试
make e2e-oss
```

E2E测试会：
1. 启动Cloud服务器
2. 启动Agent
3. 等待Agent注册和心跳
4. 创建作业
5. 验证作业完成
6. 验证输出文件存在

### 3. 手动测试

#### 测试文件扩展名保留功能

1. **准备测试文件**:
   ```powershell
   # 创建一个测试图片文件
   # (使用你现有的图片文件，例如: 281.jpg)
   ```

2. **启动服务器**:
   ```powershell
   cd cloud
   go run cmd/server/main.go -addr :8080 -dev
   ```

3. **启动Agent**:
   ```powershell
   cd agent
   go run cmd/agent/main.go -server ws://localhost:8080/wss -agent-id test-agent -agent-token dev-token
   ```

4. **创建作业** (使用curl或Postman):
   ```powershell
   $body = @{
       input_bucket = "your-bucket"
       input_key = "zfc_files/ui_tap/281.jpg"
       output_bucket = "your-bucket"
       output_extension = "json"
       command = "F:\PyEnvs\TapPredEnv\Scripts\python.exe C:\Users\lcada\PycharmProjects\TapPredProject\component_detection\PyYoloComponentDetectOpe.py {input} -m C:\Users\lcada\PycharmProjects\TapPredProject\component_detection\runs\detect\ui_component_detection_run3\weights\best.pt"
   } | ConvertTo-Json

   Invoke-RestMethod -Uri "http://localhost:8080/api/jobs" -Method POST -Body $body -ContentType "application/json"
   ```

5. **验证**:
   - 检查Agent日志，确认临时文件名包含 `.jpg` 扩展名
   - 检查作业状态，确认作业成功完成
   - 验证Python脚本能够正确识别文件类型

## 编写新测试

### 添加单元测试

#### Agent测试示例

```go
func TestClient_NewFeature(t *testing.T) {
    // 1. 设置测试环境
    client := New("ws://test", "test-agent", "test-token", 1)
    client.httpClient = &http.Client{Timeout: 5 * time.Second}
    
    // 2. 创建mock服务器（如果需要）
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 处理请求
    }))
    defer server.Close()
    
    // 3. 执行测试
    result, err := client.someMethod()
    
    // 4. 验证结果
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
}
```

#### Gateway测试示例

```go
func TestGateway_NewFeature(t *testing.T) {
    // 1. 设置mock依赖
    mockReg := newMockRegistry()
    mockStore := newMockJobStore()
    mockQueue := newMockQueue()
    mockOSS := newMockOSSProvider()
    gw := New(mockReg, mockStore, mockQueue, mockOSS, true)
    
    // 2. 准备测试数据
    agentID := "test-agent"
    mockReg.Register(agentID, "test-host", 1)
    
    // 3. 执行测试
    // ...
    
    // 4. 验证结果
    // ...
}
```

### 测试文件扩展名保留功能

已添加的测试: `TestClient_DownloadInputToFile_ExtensionPreservation`

这个测试验证：
- 各种文件扩展名（.jpg, .png, .json等）都能正确保留
- 没有扩展名的文件不会添加扩展名
- 多个点的路径（如.tar.gz）能正确提取最后一个扩展名
- 文件名格式正确：`job_{job_id}_input{.<ext>}`

### 测试最佳实践

1. **测试命名**: 使用 `TestPackage_Function_Scenario` 格式
2. **测试组织**: 每个功能应该有多个测试用例覆盖不同场景
3. **清理资源**: 使用 `defer` 清理临时文件和资源
4. **Mock外部依赖**: 使用 `httptest` 等工具mock HTTP请求
5. **并行测试**: 使用 `t.Parallel()` 加速测试（注意避免共享状态）
6. **表驱动测试**: 对于多个相似测试用例，使用表驱动方式

示例（表驱动测试）:
```go
func TestExtensionExtraction(t *testing.T) {
    testCases := []struct {
        name     string
        inputKey string
        wantExt  string
    }{
        {"jpg file", "image.jpg", ".jpg"},
        {"png file", "image.png", ".png"},
        {"no extension", "data", ""},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            ext := extractExtension(tc.inputKey)
            if ext != tc.wantExt {
                t.Errorf("Expected %s, got %s", tc.wantExt, ext)
            }
        })
    }
}
```

## 调试测试

### 运行单个测试并查看详细输出

```powershell
# 运行特定测试并显示详细输出
go test -v -run TestClient_DownloadInputToFile_ExtensionPreservation ./agent/internal/client

# 运行测试并显示覆盖率
go test -cover ./agent/internal/client

# 生成覆盖率报告
go test -coverprofile=coverage.out ./agent/internal/client
go tool cover -html=coverage.out
```

### 使用调试器

**VS Code:**
1. 在测试函数中设置断点
2. 点击"Run Test"按钮旁边的调试图标
3. 或使用 `F5` 启动调试

**命令行:**
```powershell
# 使用delve调试器
dlv test ./agent/internal/client -- -test.run TestClient_DownloadInputToFile_ExtensionPreservation
```

## 持续集成

### GitHub Actions (推荐)

创建 `.github/workflows/test.yml`:

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Generate proto
        run: make proto
      - name: Run tests
        run: make test
      - name: Run e2e
        run: make e2e
```

## 常见问题

### Q: 测试失败，提示找不到proto文件？

A: 确保先运行 `make proto` 或 `powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 proto` 生成protobuf代码。

### Q: 测试超时？

A: 检查是否有goroutine泄漏或死锁。使用 `-timeout` 参数增加超时时间：
```powershell
go test -timeout 30s ./...
```

### Q: 如何测试WebSocket连接？

A: 使用 `gorilla/websocket` 的测试工具或创建mock WebSocket连接。参考 `gateway_test.go` 中的示例。

### Q: 如何测试OSS相关功能？

A: 使用mock OSS provider（参考 `gateway_test.go` 中的 `newMockOSSProvider`）或使用MinIO进行本地测试。

## 测试覆盖率目标

- **单元测试覆盖率**: > 80%
- **关键路径覆盖率**: 100%
- **错误处理覆盖率**: > 90%

查看覆盖率:
```powershell
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```
