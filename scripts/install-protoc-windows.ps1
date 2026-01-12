# Helper script to download and setup protoc on Windows
# This is a convenience script - you can also download manually

Write-Host "Protocol Buffers Compiler Setup for Windows" -ForegroundColor Green
Write-Host ""
Write-Host "Option 1: Download manually" -ForegroundColor Yellow
Write-Host "1. Visit: https://github.com/protocolbuffers/protobuf/releases"
Write-Host "2. Download the latest protoc-*-win64.zip"
Write-Host "3. Extract to a folder (e.g., C:\tools\protoc)"
Write-Host "4. Add the bin folder to your PATH environment variable"
Write-Host ""
Write-Host "Option 2: Use Chocolatey (if installed)" -ForegroundColor Yellow
Write-Host "choco install protoc"
Write-Host ""
Write-Host "Option 3: Use Scoop (if installed)" -ForegroundColor Yellow
Write-Host "scoop install protoc"
Write-Host ""
Write-Host "After installation, verify with: protoc --version" -ForegroundColor Green
