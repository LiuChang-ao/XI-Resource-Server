# PowerShell build script for Windows

param(
    [Parameter(Mandatory=$false)]
    [ValidateSet("proto", "build-cloud", "build-agent", "e2e", "e2e-oss", "test", "clean", "all")]
    [string]$Target = "all"
)

$ErrorActionPreference = "Stop"

# Refresh environment variables to pick up newly added PATH entries
$env:PATH = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

# Change to project root directory (parent of scripts directory)
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
Push-Location $projectRoot

function Invoke-Protoc {
    Write-Host "Generating protobuf code..." -ForegroundColor Green
    Push-Location proto
    try {
        protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
        if ($LASTEXITCODE -ne 0) {
            throw "protoc failed with exit code $LASTEXITCODE"
        }
        Write-Host "Protobuf code generated successfully" -ForegroundColor Green
    } finally {
        Pop-Location
    }
}

function Build-Cloud {
    Write-Host "Building cloud server..." -ForegroundColor Green
    Push-Location cloud
    try {
        go build -ldflags="-s -w" -o ../bin/server.exe ./cmd/server
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
        Write-Host "Cloud server built successfully" -ForegroundColor Green
    } finally {
        Pop-Location
    }
}

function Build-Agent {
    Write-Host "Building agent..." -ForegroundColor Green
    Push-Location agent
    try {
        go build -ldflags="-s -w" -o ../bin/agent.exe ./cmd/agent
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
        Write-Host "Agent built successfully" -ForegroundColor Green
    } finally {
        Pop-Location
    }
}

function Invoke-E2E {
    Write-Host "Running e2e test (M0)..." -ForegroundColor Green
    Push-Location scripts
    try {
        go run M0_e2e.go
        if ($LASTEXITCODE -ne 0) {
            throw "e2e test failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
}

function Invoke-E2EOSS {
    Write-Host "Running e2e test with real OSS..." -ForegroundColor Green
    Push-Location scripts
    try {
        go run e2e_oss.go
        if ($LASTEXITCODE -ne 0) {
            throw "e2e-oss test failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
}

function Invoke-Test {
    Write-Host "Running unit tests..." -ForegroundColor Green
    
    # Ensure go.sum is up to date
    Write-Host "Updating dependencies..." -ForegroundColor Yellow
    Push-Location cloud
    try {
        go mod tidy
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "go mod tidy failed, but continuing..."
        }
    } finally {
        Pop-Location
    }
    
    Push-Location proto
    try {
        go mod tidy
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "go mod tidy failed, but continuing..."
        }
    } finally {
        Pop-Location
    }
    
    # Run tests with CGO enabled (required for SQLite)
    Push-Location cloud
    try {
        $env:CGO_ENABLED = "1"
        go test ./...
        if ($LASTEXITCODE -ne 0) {
            throw "cloud tests failed"
        }
    } finally {
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
        Pop-Location
    }
    
    Push-Location agent
    try {
        $env:CGO_ENABLED = "1"
        go test ./...
        if ($LASTEXITCODE -ne 0) {
            throw "agent tests failed"
        }
    } finally {
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
        Pop-Location
    }
}

function Invoke-Clean {
    Write-Host "Cleaning build artifacts..." -ForegroundColor Green
    if (Test-Path bin) {
        Remove-Item -Recurse -Force bin
    }
    if (Test-Path proto/control) {
        Remove-Item -Recurse -Force proto/control
    }
    Write-Host "Clean completed" -ForegroundColor Green
}

# Create bin directory if it doesn't exist
if (-not (Test-Path bin)) {
    New-Item -ItemType Directory -Path bin | Out-Null
}

switch ($Target) {
    "proto" { Invoke-Protoc }
    "build-cloud" { Invoke-Protoc; Build-Cloud }
    "build-agent" { Invoke-Protoc; Build-Agent }
    "e2e" { Invoke-Protoc; Invoke-E2E }
    "e2e-oss" { Invoke-Protoc; Build-Cloud; Build-Agent; Invoke-E2EOSS }
    "test" { Invoke-Test }
    "clean" { Invoke-Clean }
    "all" { 
        Invoke-Protoc
        Build-Cloud
        Build-Agent
    }
}

Pop-Location
