# Agent 启动脚本
# 使用方法: .\start-agent.ps1 -Server "wss://your-server.com/wss" -AgentID "workstation-001" -AgentToken "your-token"

param(
    [Parameter(Mandatory=$false)]
    [string]$Server = "ws://116.62.112.11:8086/wss",
    
    [Parameter(Mandatory=$true)]
    [string]$AgentID = "agent-liuPC",
    
    [Parameter(Mandatory=$false)]
    [string]$AgentToken = "dev-token",
    
    [Parameter(Mandatory=$false)]
    [int]$MaxConcurrency = 1
)

# 获取脚本所在目录的父目录（项目根目录）
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$agentExe = Join-Path $projectRoot "bin\agent.exe"

# 检查 agent.exe 是否存在
if (-not (Test-Path $agentExe)) {
    Write-Host "错误: 找不到 agent.exe" -ForegroundColor Red
    Write-Host "路径: $agentExe" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "请先编译 agent:" -ForegroundColor Yellow
    Write-Host "  powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-agent" -ForegroundColor Cyan
    exit 1
}

# 显示配置信息
Write-Host "========================================" -ForegroundColor Green
Write-Host "启动 XI Resource Agent" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "服务器:     $Server" -ForegroundColor Cyan
Write-Host "Agent ID:    $AgentID" -ForegroundColor Cyan
Write-Host "Token:       $AgentToken" -ForegroundColor Cyan
Write-Host "最大并发:   $MaxConcurrency" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Green
Write-Host ""

# 运行 agent
& $agentExe `
  -server $Server `
  -agent-id $AgentID `
  -agent-token $AgentToken `
  -max-concurrency $MaxConcurrency

# 如果 agent 退出，显示退出信息
if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "Agent 异常退出，退出代码: $LASTEXITCODE" -ForegroundColor Red
}
