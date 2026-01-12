# 快速刷新 PowerShell 环境变量（无需重启系统）

## 方法 1: 使用刷新脚本（推荐）

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\refresh-env.ps1
```

## 方法 2: 手动刷新（一行命令）

在当前 PowerShell 会话中运行：

```powershell
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User"); $env:CGO_ENABLED = "1"
```

## 方法 3: 如果安装了 Chocolatey

```powershell
refreshenv
```

## 验证

刷新后验证：

```powershell
# 检查 GCC
gcc --version

# 检查 CGO
go env CGO_ENABLED

# 运行测试
cd cloud
go test ./internal/job -v
```

## 重要提示

如果测试仍然失败，可能需要重新编译依赖：

```powershell
# 清理缓存并重新安装依赖
go clean -cache
go mod tidy

# 然后运行测试
go test ./internal/job -v
```

## 永久设置（可选）

如果希望每次打开 PowerShell 都自动设置，可以添加到 PowerShell 配置文件：

```powershell
# 查看配置文件路径
$PROFILE

# 编辑配置文件，添加：
$env:CGO_ENABLED = "1"
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
```
