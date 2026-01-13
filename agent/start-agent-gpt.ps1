﻿# Agent 启动脚本（支持默认参数直接启动，兼容 Windows PowerShell 5.1）
# 用法（均为可选参数）:
#   .\start-agent-gpt.ps1
#   .\start-agent-gpt.ps1 -Server "wss://your-server.com/wss" -AgentID "workstation-001" -AgentToken "your-token" -MaxConcurrency 2
#
# 也可通过环境变量覆盖默认值（可选）:
#   XI_AGENT_SERVER
#   XI_AGENT_ID
#   XI_AGENT_TOKEN
#   XI_AGENT_MAX_CONCURRENCY
#
# 注意：PowerShell 的 param(...) 必须出现在脚本最前（除注释/#requires 之外）。

param(
    [Parameter(Mandatory=$false)]
    [string]$Server,

    [Parameter(Mandatory=$false)]
    [string]$AgentID,

    [Parameter(Mandatory=$false)]
    [string]$AgentToken,

    [Parameter(Mandatory=$false)]
    [ValidateRange(1, 1024)]
    [int]$MaxConcurrency = 1,

    # 显示完整 Token（默认脱敏显示，避免日志泄露）
    [switch]$ShowToken
)

# ---- 处理中文输出（避免控制台乱码）----
try {
    if ($env:OS -eq 'Windows_NT') {
        # 将当前控制台代码页切换到 UTF-8（在 Windows PowerShell 5.1 / 旧控制台上尤其有用）
        chcp 65001 | Out-Null
    }
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [Console]::OutputEncoding = $utf8NoBom
    $OutputEncoding = $utf8NoBom

    # 让 Out-File/Set-Content/Add-Content 默认使用 UTF-8，避免写文件时中文乱码
    $PSDefaultParameterValues['Out-File:Encoding']    = 'utf8'
    $PSDefaultParameterValues['Set-Content:Encoding'] = 'utf8'
    $PSDefaultParameterValues['Add-Content:Encoding'] = 'utf8'
} catch {
    # 忽略编码设置失败（例如受限环境）
}

# ---- 默认值（与原脚本保持一致）----
$defaultServer         = "ws://116.62.112.11:8086/wss"
$defaultAgentID        = "agent-liuPC"
$defaultAgentToken     = "dev-token"
$defaultMaxConcurrency = 1

# ---- 仅当用户未显式传参时，才从环境变量/默认值填充 ----
if (-not $PSBoundParameters.ContainsKey('Server') -or [string]::IsNullOrWhiteSpace($Server)) {
    $Server = if ($env:XI_AGENT_SERVER) { $env:XI_AGENT_SERVER } else { $defaultServer }
}

if (-not $PSBoundParameters.ContainsKey('AgentID') -or [string]::IsNullOrWhiteSpace($AgentID)) {
    $AgentID = if ($env:XI_AGENT_ID) { $env:XI_AGENT_ID } else { $defaultAgentID }
}

if (-not $PSBoundParameters.ContainsKey('AgentToken') -or [string]::IsNullOrWhiteSpace($AgentToken)) {
    $AgentToken = if ($env:XI_AGENT_TOKEN) { $env:XI_AGENT_TOKEN } else { $defaultAgentToken }
}

if (-not $PSBoundParameters.ContainsKey('MaxConcurrency')) {
    $mc = $env:XI_AGENT_MAX_CONCURRENCY -as [int]
    if ($null -ne $mc -and $mc -ge 1) {
        $MaxConcurrency = $mc
    } else {
        $MaxConcurrency = $defaultMaxConcurrency
    }
}

# 获取脚本所在目录的父目录（项目根目录）
$scriptDir = $PSScriptRoot
if (-not $scriptDir) {
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
}
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

# Token 显示（默认脱敏）
$tokenDisplay = if ($ShowToken) {
    $AgentToken
} elseif ([string]::IsNullOrEmpty($AgentToken)) {
    "(empty)"
} elseif ($AgentToken.Length -le 8) {
    "********"
} else {
    $AgentToken.Substring(0,4) + "..." + $AgentToken.Substring($AgentToken.Length-4)
}

# 显示配置信息
Write-Host "========================================" -ForegroundColor Green
Write-Host "启动 XI Resource Agent" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ("服务器:     {0}" -f $Server) -ForegroundColor Cyan
Write-Host ("Agent ID:    {0}" -f $AgentID) -ForegroundColor Cyan
Write-Host ("Token:       {0}" -f $tokenDisplay) -ForegroundColor Cyan
Write-Host ("最大并发:   {0}" -f $MaxConcurrency) -ForegroundColor Cyan
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
