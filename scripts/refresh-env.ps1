# Refresh Environment Variables in PowerShell
# This script reloads PATH and other environment variables without restarting the system

Write-Host "=== Refreshing Environment Variables ===" -ForegroundColor Cyan

# Method 1: Reload PATH from registry
Write-Host "`n1. Reloading PATH from registry..." -ForegroundColor Yellow
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")

Write-Host "   ✓ PATH refreshed" -ForegroundColor Green

# Method 2: Reload CGO_ENABLED if set
$cgoUser = [System.Environment]::GetEnvironmentVariable("CGO_ENABLED", "User")
$cgoMachine = [System.Environment]::GetEnvironmentVariable("CGO_ENABLED", "Machine")

if ($cgoUser) {
    $env:CGO_ENABLED = $cgoUser
    Write-Host "   ✓ CGO_ENABLED set to: $cgoUser" -ForegroundColor Green
} elseif ($cgoMachine) {
    $env:CGO_ENABLED = $cgoMachine
    Write-Host "   ✓ CGO_ENABLED set to: $cgoMachine" -ForegroundColor Green
}

# Verify GCC is now available
Write-Host "`n2. Verifying GCC..." -ForegroundColor Yellow
try {
    $gccVersion = gcc --version 2>&1 | Select-Object -First 1
    Write-Host "   ✓ GCC found: $gccVersion" -ForegroundColor Green
} catch {
    Write-Host "   ✗ GCC still not found" -ForegroundColor Red
    Write-Host "`n   Troubleshooting:" -ForegroundColor Yellow
    Write-Host "   - Make sure GCC was added to PATH during installation" -ForegroundColor White
    Write-Host "   - Check if GCC is in: C:\TDM-GCC-64\bin (or similar)" -ForegroundColor White
    Write-Host "   - Try manually adding: `$env:PATH += ';C:\TDM-GCC-64\bin'" -ForegroundColor White
}

# Verify CGO
Write-Host "`n3. Verifying CGO..." -ForegroundColor Yellow
$cgo = go env CGO_ENABLED
Write-Host "   CGO_ENABLED = $cgo" -ForegroundColor $(if ($cgo -eq "1") { "Green" } else { "Yellow" })

if ($cgo -ne "1") {
    Write-Host "   Setting CGO_ENABLED=1..." -ForegroundColor Yellow
    $env:CGO_ENABLED = "1"
}

Write-Host "`n=== Refresh Complete ===" -ForegroundColor Cyan
Write-Host "`nYou can now run tests:" -ForegroundColor Yellow
Write-Host "  cd cloud" -ForegroundColor White
Write-Host "  go test ./internal/job -v" -ForegroundColor White
