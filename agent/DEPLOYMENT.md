# Agent 部署指南 (Windows)

本文档说明如何在 Windows 环境下编译 agent 可执行文件并运行。

## 0. 前置依赖安装

### 0.1 安装 Go (如果未安装)

```powershell
# 检查 Go 版本
go version

# 如果未安装，请先安装 Go 1.21 或更高版本
# 下载地址: https://go.dev/dl/
# 下载 Windows 安装程序并运行
```

### 0.2 安装 Protocol Buffers 编译器

```powershell
# 下载 protoc
# 访问: https://github.com/protocolbuffers/protobuf/releases
# 下载 protoc-<version>-win64.zip
# 解压后将 bin 目录添加到 PATH

# 验证安装
protoc --version
```

### 0.3 安装 protoc-gen-go 插件

```powershell
# 安装 Go protobuf 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# 确保 Go 的 bin 目录在 PATH 中
# 检查 Go 环境变量
go env GOPATH
# 默认路径通常是: C:\Users\<用户名>\go\bin

# 将 Go bin 目录添加到 PATH (如果不在 PATH 中)
# 在 PowerShell 中临时添加:
$env:PATH += ";$env:USERPROFILE\go\bin"

# 永久添加到 PATH:
# 1. 打开"系统属性" -> "环境变量"
# 2. 编辑用户变量 "Path"
# 3. 添加: C:\Users\<用户名>\go\bin

# 验证 protoc-gen-go 是否安装成功
protoc-gen-go --version
```

## 1. 编译 Agent

### 方法 1: 使用构建脚本 (推荐)

```powershell
# 在项目根目录执行
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-agent
```

编译完成后，可执行文件位于 `bin\agent.exe`。

### 方法 2: 手动编译

```powershell
# 1. 生成 protobuf 代码
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
cd ..

# 2. 编译 agent
cd agent
go build -ldflags="-s -w" -o ../bin/agent.exe ./cmd/agent
cd ..
```

### 方法 3: 跨平台编译 (在 Linux/macOS 上编译 Windows 版本)

```bash
# 在 Linux/macOS 上编译 Windows 版本
cd agent
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ../bin/agent.exe ./cmd/agent
cd ..
```

## 2. 运行 Agent

### 2.1 命令行参数

Agent 支持以下命令行参数：

| 参数 | 说明 | 默认值 | 必需 |
|------|------|--------|------|
| `-server` | 服务器 WebSocket URL | `ws://localhost:8080/wss` | 否 |
| `-agent-id` | Agent ID (唯一标识符) | 无 | **是** |
| `-agent-token` | Agent 认证令牌 | `dev-token` | 否 |
| `-max-concurrency` | 最大并发任务数 | `1` | 否 |

### 2.2 基本运行示例

```powershell
# 开发环境 (本地测试)
.\bin\agent.exe `
  -server ws://localhost:8080/wss `
  -agent-id workstation-001 `
  -agent-token dev-token

# 生产环境 (使用 WSS)
.\bin\agent.exe `
  -server wss://your-server.com/wss `
  -agent-id workstation-001 `
  -agent-token your-secure-token `
  -max-concurrency 2
```

### 2.3 使用环境变量 (可选)

也可以通过环境变量设置参数：

```powershell
# 设置环境变量
$env:AGENT_SERVER = "wss://your-server.com/wss"
$env:AGENT_ID = "workstation-001"
$env:AGENT_TOKEN = "your-secure-token"
$env:AGENT_MAX_CONCURRENCY = "2"

# 运行 agent (需要修改代码支持环境变量，或使用包装脚本)
.\bin\agent.exe
```

### 2.4 后台运行 (使用 PowerShell 后台作业)

```powershell
# 启动后台作业
$job = Start-Job -ScriptBlock {
    Set-Location "D:\path\to\XI-Resource-Server"
    .\bin\agent.exe `
      -server wss://your-server.com/wss `
      -agent-id workstation-001 `
      -agent-token your-secure-token
}

# 查看作业状态
Get-Job

# 查看输出
Receive-Job -Job $job

# 停止作业
Stop-Job -Job $job
Remove-Job -Job $job
```

### 2.5 作为 Windows 服务运行 (推荐生产环境)

#### 使用 NSSM (Non-Sucking Service Manager)

1. **下载 NSSM**
   - 访问: https://nssm.cc/download
   - 下载最新版本并解压

2. **安装服务**
   ```powershell
   # 以管理员身份运行 PowerShell
   cd C:\path\to\nssm\win64
   
   .\nssm.exe install XIResourceAgent `
     "D:\path\to\XI-Resource-Server\bin\agent.exe" `
     "-server wss://your-server.com/wss -agent-id workstation-001 -agent-token your-secure-token"
   ```

3. **配置服务**
   ```powershell
   # 设置服务描述
   .\nssm.exe set XIResourceAgent Description "XI Resource Agent Service"
   
   # 设置启动类型为自动
   .\nssm.exe set XIResourceAgent Start SERVICE_AUTO_START
   
   # 设置工作目录
   .\nssm.exe set XIResourceAgent AppDirectory "D:\path\to\XI-Resource-Server"
   ```

4. **管理服务**
   ```powershell
   # 启动服务
   .\nssm.exe start XIResourceAgent
   
   # 停止服务
   .\nssm.exe stop XIResourceAgent
   
   # 查看服务状态
   .\nssm.exe status XIResourceAgent
   
   # 查看服务日志
   .\nssm.exe status XIResourceAgent
   
   # 卸载服务
   .\nssm.exe remove XIResourceAgent confirm
   ```

#### 使用 sc.exe (Windows 内置)

```powershell
# 以管理员身份运行 PowerShell

# 创建服务
sc.exe create XIResourceAgent `
  binPath= "D:\path\to\XI-Resource-Server\bin\agent.exe -server wss://your-server.com/wss -agent-id workstation-001 -agent-token your-secure-token" `
  start= auto `
  DisplayName= "XI Resource Agent"

# 启动服务
sc.exe start XIResourceAgent

# 停止服务
sc.exe stop XIResourceAgent

# 删除服务
sc.exe delete XIResourceAgent
```

## 3. 配置示例

### 3.1 开发环境配置

```powershell
.\bin\agent.exe `
  -server ws://localhost:8080/wss `
  -agent-id dev-agent-001 `
  -agent-token dev-token `
  -max-concurrency 1
```

### 3.2 生产环境配置

```powershell
.\bin\agent.exe `
  -server wss://cloud.example.com/wss `
  -agent-id workstation-rtx4090-01 `
  -agent-token "your-secure-production-token" `
  -max-concurrency 2
```

### 3.3 多 Agent 配置 (同一台机器运行多个 agent)

```powershell
# Agent 1
.\bin\agent.exe `
  -server wss://cloud.example.com/wss `
  -agent-id workstation-01 `
  -agent-token token-01 `
  -max-concurrency 1

# Agent 2 (在另一个终端或服务中)
.\bin\agent.exe `
  -server wss://cloud.example.com/wss `
  -agent-id workstation-02 `
  -agent-token token-02 `
  -max-concurrency 1
```

## 4. 验证 Agent 运行状态

### 4.1 检查 Agent 是否在线

```powershell
# 使用 curl (如果安装了)
curl http://your-server.com:8080/api/agents/online

# 或使用 PowerShell
Invoke-RestMethod -Uri "http://your-server.com:8080/api/agents/online"
```

应该返回包含你的 agent 信息的 JSON 数组。

### 4.2 查看 Agent 日志

如果 agent 在前台运行，日志会直接输出到控制台。

如果作为服务运行，查看服务日志：

```powershell
# 使用 NSSM
Get-Content "C:\nssm\service\XIResourceAgent\stdout.log" -Tail 50
Get-Content "C:\nssm\service\XIResourceAgent\stderr.log" -Tail 50

# 或使用 Windows 事件查看器
eventvwr.msc
# 查看 "Windows 日志" -> "应用程序"
```

## 5. 故障排查

### 5.1 Agent 无法连接到服务器

**检查项：**
1. 服务器地址是否正确
2. 网络连接是否正常
3. 防火墙是否阻止连接
4. 服务器是否正在运行

**测试连接：**
```powershell
# 测试 WebSocket 连接 (需要安装测试工具)
# 或使用浏览器访问服务器健康检查端点
Invoke-RestMethod -Uri "http://your-server.com:8080/health"
```

### 5.2 Agent ID 冲突

如果多个 agent 使用相同的 `agent-id`，服务器可能会拒绝连接。确保每个 agent 使用唯一的 ID。

### 5.3 Token 认证失败

确保 `-agent-token` 参数与服务器端配置的 token 匹配。

### 5.4 Agent 频繁断开重连

可能的原因：
1. 网络不稳定
2. 服务器负载过高
3. 心跳超时设置过短

检查服务器日志和 agent 日志以确定具体原因。

## 6. 安全建议

1. **使用强密码作为 agent-token**
   - 生产环境不要使用 `dev-token`
   - 使用随机生成的强密码

2. **使用 WSS (WebSocket Secure)**
   - 生产环境必须使用 `wss://` 而不是 `ws://`
   - 确保服务器配置了有效的 TLS 证书

3. **限制网络访问**
   - 如果可能，限制 agent 只能访问指定的服务器地址

4. **定期更新 agent**
   - 定期更新 agent 可执行文件以获取安全补丁

5. **监控 agent 运行状态**
   - 设置监控系统检查 agent 是否在线
   - 配置告警以便及时发现问题

## 7. 完整部署检查清单

- [ ] 已安装 Go 1.21 或更高版本
- [ ] 已安装 protoc 和 protoc-gen-go
- [ ] 已生成 protobuf 代码
- [ ] 已编译 agent.exe
- [ ] 已配置服务器地址和认证信息
- [ ] 已测试 agent 连接
- [ ] 已配置为 Windows 服务 (生产环境)
- [ ] 已设置监控和告警
- [ ] 已配置日志记录

## 8. 快速启动脚本示例

创建一个 `start-agent.ps1` 脚本：

```powershell
# start-agent.ps1
param(
    [string]$Server = "wss://your-server.com/wss",
    [string]$AgentID = "workstation-001",
    [string]$AgentToken = "your-token",
    [int]$MaxConcurrency = 1
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$agentExe = Join-Path $scriptDir "bin\agent.exe"

if (-not (Test-Path $agentExe)) {
    Write-Host "错误: 找不到 agent.exe，请先编译" -ForegroundColor Red
    exit 1
}

Write-Host "启动 Agent..." -ForegroundColor Green
Write-Host "  服务器: $Server" -ForegroundColor Cyan
Write-Host "  Agent ID: $AgentID" -ForegroundColor Cyan
Write-Host "  最大并发: $MaxConcurrency" -ForegroundColor Cyan

& $agentExe `
  -server $Server `
  -agent-id $AgentID `
  -agent-token $AgentToken `
  -max-concurrency $MaxConcurrency
```

使用方法：
```powershell
.\start-agent.ps1 -Server "wss://your-server.com/wss" -AgentID "workstation-001" -AgentToken "your-token"
```
