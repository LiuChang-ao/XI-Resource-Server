# OSS Module Testing Guide

## 测试文件说明

`oss_test.go` 包含以下测试：

### 需要真实 COS 凭证的测试
1. **TestCOSProvider_GenerateDownloadURL** - 测试生成下载 Presigned URL
2. **TestCOSProvider_GenerateUploadURL** - 测试生成上传 Presigned URL
3. **TestCOSProvider_GenerateUploadURLWithPrefix** - 测试使用前缀生成上传 URL

### 不需要真实凭证的测试（单元测试）
4. **TestCOSProvider_Validation** - 测试配置验证逻辑
5. **TestCOSProvider_DefaultTTL** - 测试默认 TTL 设置

## 运行测试

### 方法 1: 使用 .env 文件（推荐）

1. 在 `cloud/` 目录或项目根目录创建 `.env` 文件：

```bash
COS_SECRET_ID=your-secret-id
COS_SECRET_KEY=your-secret-key
COS_BUCKET=your-bucket
COS_REGION=ap-shanghai
```

2. 运行所有测试：

```powershell
cd cloud
go test ./internal/oss -v
```

### 方法 2: 使用环境变量

**PowerShell:**
```powershell
$env:COS_SECRET_ID = "your-secret-id"
$env:COS_SECRET_KEY = "your-secret-key"
$env:COS_BUCKET = "your-bucket"
$env:COS_REGION = "ap-shanghai"
cd cloud
go test ./internal/oss -v
```

**Bash:**
```bash
export COS_SECRET_ID=your-secret-id
export COS_SECRET_KEY=your-secret-key
export COS_BUCKET=your-bucket
export COS_REGION=ap-shanghai
cd cloud
go test ./internal/oss -v
```

### 方法 3: 只运行不需要凭证的测试

如果不想设置真实凭证，可以只运行单元测试：

```powershell
cd cloud
go test ./internal/oss -run "TestCOSProvider_Validation|TestCOSProvider_DefaultTTL" -v
```

## 运行特定测试

### 运行单个测试：

```powershell
# 测试下载 URL 生成
go test ./internal/oss -run TestCOSProvider_GenerateDownloadURL -v

# 测试上传 URL 生成
go test ./internal/oss -run TestCOSProvider_GenerateUploadURL -v

# 测试配置验证
go test ./internal/oss -run TestCOSProvider_Validation -v
```

### 运行所有测试（包括跳过的）：

```powershell
go test ./internal/oss -v -count=1
```

## 测试行为说明

### 有凭证时
- 所有 5 个测试都会运行
- 前 3 个测试会实际生成 Presigned URL 并验证其格式
- 测试输出会显示生成的 URL（用于调试）

### 无凭证时
- 前 3 个测试会被跳过（`t.Skip`）
- 后 2 个测试会正常运行（不需要真实凭证）
- 测试结果会显示跳过的测试数量

## 示例输出

**有凭证时：**
```
=== RUN   TestCOSProvider_GenerateDownloadURL
    oss_test.go:46: Generated download URL: https://...
--- PASS: TestCOSProvider_GenerateDownloadURL (0.05s)
=== RUN   TestCOSProvider_GenerateUploadURL
    oss_test.go:85: Generated upload URL: https://...
--- PASS: TestCOSProvider_GenerateUploadURL (0.03s)
...
PASS
ok      github.com/xiresource/cloud/internal/oss    0.150s
```

**无凭证时：**
```
=== RUN   TestCOSProvider_GenerateDownloadURL
    oss_test.go:18: Skipping test: COS credentials not set...
--- SKIP: TestCOSProvider_GenerateDownloadURL (0.00s)
=== RUN   TestCOSProvider_Validation
--- PASS: TestCOSProvider_Validation (0.00s)
...
PASS
ok      github.com/xiresource/cloud/internal/oss    0.050s
```

## 注意事项

1. **不要提交真实凭证**：`.env` 文件已在 `.gitignore` 中，确保不会提交到版本控制
2. **测试不会实际访问 COS**：测试只验证 URL 生成，不会实际下载或上传文件
3. **URL 有效期**：生成的 Presigned URL 默认有效期为 15 分钟，可以在测试中使用
4. **测试键名**：测试使用的键名（如 `test/input.txt`）不需要在 COS 中实际存在

## 故障排查

### 测试被跳过
- 检查环境变量是否正确设置
- 确认 `.env` 文件在正确的位置（`cloud/` 或项目根目录）
- 验证 `.env` 文件格式是否正确（无引号，无空格）

### 测试失败
- 检查 COS 凭证是否正确
- 确认 bucket 和 region 配置正确
- 查看详细错误信息：`go test ./internal/oss -v`
