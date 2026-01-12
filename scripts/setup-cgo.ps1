# Setup CGO for Windows
# This script helps verify and configure CGO support

Write-Host "=== CGO Setup Check ===" -ForegroundColor Cyan

# Check if GCC is available
Write-Host "`n1. Checking for C compiler (GCC)..." -ForegroundColor Yellow
try {
    $gccVersion = gcc --version 2>&1 | Select-Object -First 1
    Write-Host "   ✓ Found: $gccVersion" -ForegroundColor Green
} catch {
    Write-Host "   ✗ GCC not found in PATH" -ForegroundColor Red
    Write-Host "`n   Please install a C compiler:" -ForegroundColor Yellow
    Write-Host "   - MSYS2: https://www.msys2.org/" -ForegroundColor White
    Write-Host "   - TDM-GCC: https://jmeubank.github.io/tdm-gcc/" -ForegroundColor White
    Write-Host "   - Or use: choco install mingw" -ForegroundColor White
    exit 1
}

# Check CGO_ENABLED
Write-Host "`n2. Checking CGO_ENABLED..." -ForegroundColor Yellow
$cgoEnv = $env:CGO_ENABLED
$cgoGo = go env CGO_ENABLED

Write-Host "   Environment variable: $cgoEnv" -ForegroundColor $(if ($cgoEnv -eq "1") { "Green" } else { "Yellow" })
Write-Host "   Go env value: $cgoGo" -ForegroundColor $(if ($cgoGo -eq "1") { "Green" } else { "Yellow" })

if ($cgoGo -ne "1") {
    Write-Host "`n   Setting CGO_ENABLED=1 for this session..." -ForegroundColor Yellow
    $env:CGO_ENABLED = "1"
    Write-Host "   ✓ CGO enabled" -ForegroundColor Green
} else {
    Write-Host "   ✓ CGO is already enabled" -ForegroundColor Green
}

# Test CGO compilation
Write-Host "`n3. Testing CGO compilation..." -ForegroundColor Yellow
$testFile = [System.IO.Path]::GetTempFileName() + ".go"
@"
package main

/*
#include <stdio.h>
void hello() {
    printf("Hello from C!\n");
}
*/
import "C"

func main() {
    C.hello()
}
"@ | Out-File -FilePath $testFile -Encoding UTF8

try {
    $testDir = Split-Path $testFile
    Push-Location $testDir
    go build -o test-cgo.exe $testFile 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "   ✓ CGO compilation successful" -ForegroundColor Green
        Remove-Item test-cgo.exe -ErrorAction SilentlyContinue
    } else {
        Write-Host "   ✗ CGO compilation failed" -ForegroundColor Red
    }
} catch {
    Write-Host "   ✗ Error: $_" -ForegroundColor Red
} finally {
    Pop-Location
    Remove-Item $testFile -ErrorAction SilentlyContinue
}

Write-Host "`n=== Setup Complete ===" -ForegroundColor Cyan
Write-Host "`nTo permanently enable CGO, run:" -ForegroundColor Yellow
Write-Host '  [System.Environment]::SetEnvironmentVariable("CGO_ENABLED", "1", "User")' -ForegroundColor White
Write-Host "`nThen restart your PowerShell session." -ForegroundColor Yellow
