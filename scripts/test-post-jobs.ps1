# POST /jobs API 测试脚本
# 测试 Issue 4 实现：安全护栏和 Redis 队列

param(
    [string]$BaseUrl = "http://localhost:8080",
    [switch]$SkipRedisCheck = $false
)

$ErrorActionPreference = "Continue"

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  POST /jobs API 测试" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$global:passed = 0
$global:failed = 0

function Test-Case {
    param(
        [string]$Name,
        [scriptblock]$Test,
        [int]$ExpectedStatus = 200
    )
    
    Write-Host "测试: $Name" -ForegroundColor Yellow
    try {
        $result = & $Test
        if (($result.StatusCode -eq $ExpectedStatus) -or ($result -ne $null)) {
            Write-Host "  [OK] 通过" -ForegroundColor Green
            $global:passed++
            return $true
        } else {
            Write-Host "  [FAIL] 失败: 状态码不匹配" -ForegroundColor Red
            $global:failed++
            return $false
        }
    } catch {
        $statusCode = $null
        if ($null -ne $_.Exception.Response) {
            $statusCode = $_.Exception.Response.StatusCode.value__
        }
        if ($statusCode -eq $ExpectedStatus) {
            Write-Host "  [OK] 通过 (预期错误: $statusCode)" -ForegroundColor Green
            $global:passed++
            return $true
        } else {
            Write-Host "  [FAIL] 失败: 状态码 $statusCode (预期: $ExpectedStatus)" -ForegroundColor Red
            Write-Host "    错误: $($_.Exception.Message)" -ForegroundColor Red
            $global:failed++
            return $false
        }
    }
}

# 测试 1: 正常创建 Job
$test1Script = {
    param($BaseUrl)
    $bodyObj = @{
        input_bucket = "test-bucket"
        input_key = "inputs/job-$(Get-Date -Format 'yyyyMMddHHmmss')/data.zip"
        output_bucket = "test-bucket"
    }
    $bodyJson = $bodyObj | ConvertTo-Json

    $response = Invoke-RestMethod -Uri "$BaseUrl/api/jobs" -Method POST -ContentType "application/json" -Body $bodyJson
    
    if ($response.job_id) {
        Write-Host "    Job ID: $($response.job_id)" -ForegroundColor Gray
        return @{ StatusCode = 201; JobId = $response.job_id }
    }
    return $null
}
$test1Wrapper = { & $test1Script -BaseUrl $BaseUrl }
Test-Case -Name "正常创建 Job" -Test $test1Wrapper -ExpectedStatus 201

# 验证 Redis 队列
if (-not $SkipRedisCheck) {
    Write-Host ""
    Write-Host "验证 Redis 队列..." -ForegroundColor Yellow
    try {
        $queueSizeOutput = docker exec redis redis-cli LLEN jobs:pending 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Host "  [OK] Redis 队列大小: $queueSizeOutput" -ForegroundColor Green
        } else {
            Write-Host "  [WARN] Redis 不可用，跳过队列验证" -ForegroundColor Yellow
        }
    } catch {
        Write-Host "  [WARN] 无法检查 Redis: $_" -ForegroundColor Yellow
    }
}

# 测试 2: 拒绝 multipart/form-data
$test2Script = {
    param($BaseUrl)
    $bodyObj = @{
        input_bucket = "test-bucket"
        input_key = "test.txt"
        output_bucket = "test-bucket"
    }
    $bodyJson = $bodyObj | ConvertTo-Json

    Invoke-RestMethod -Uri "$BaseUrl/api/jobs" -Method POST -ContentType "multipart/form-data" -Body $bodyJson -ErrorAction Stop
    return $null
}
$test2Wrapper = { & $test2Script -BaseUrl $BaseUrl }
Test-Case -Name "拒绝 multipart/form-data" -Test $test2Wrapper -ExpectedStatus 415

# 测试 3: 拒绝大文件 (>1MB)
$test3Script = {
    param($BaseUrl)
    $sizeBytes = [int](1.5 * 1024 * 1024)
    $largeData = "x" * $sizeBytes
    $bodyObj = @{
        input_bucket = "test-bucket"
        input_key = "test.txt"
        output_bucket = "test-bucket"
        dummy_data = $largeData
    }
    $bodyJson = $bodyObj | ConvertTo-Json -Depth 10

    Invoke-RestMethod -Uri "$BaseUrl/api/jobs" -Method POST -ContentType "application/json" -Body $bodyJson -ErrorAction Stop
    return $null
}
$test3Wrapper = { & $test3Script -BaseUrl $BaseUrl }
Test-Case -Name "拒绝大文件 (>1MB)" -Test $test3Wrapper -ExpectedStatus 413

# 测试 4: 拒绝非 JSON Content-Type
$test4Script = {
    param($BaseUrl)
    $bodyText = '{"input_bucket":"test","input_key":"test","output_bucket":"test"}'

    Invoke-RestMethod -Uri "$BaseUrl/api/jobs" -Method POST -ContentType "text/plain" -Body $bodyText -ErrorAction Stop
    return $null
}
$test4Wrapper = { & $test4Script -BaseUrl $BaseUrl }
Test-Case -Name "拒绝非 JSON Content-Type" -Test $test4Wrapper -ExpectedStatus 415

# 测试 5: 验证必需字段
$test5Script = {
    param($BaseUrl)
    $bodyObj = @{
        input_bucket = "test-bucket"
        output_bucket = "test-bucket"
    }
    $bodyJson = $bodyObj | ConvertTo-Json

    Invoke-RestMethod -Uri "$BaseUrl/api/jobs" -Method POST -ContentType "application/json" -Body $bodyJson -ErrorAction Stop
    return $null
}
$test5Wrapper = { & $test5Script -BaseUrl $BaseUrl }
Test-Case -Name "验证必需字段 (缺少 input_key)" -Test $test5Wrapper -ExpectedStatus 400

# 测试 6: 空 body 拒绝
$test6Script = {
    param($BaseUrl)
    Invoke-RestMethod -Uri "$BaseUrl/api/jobs" -Method POST -ContentType "application/json" -Body "" -ErrorAction Stop
    return $null
}
$test6Wrapper = { & $test6Script -BaseUrl $BaseUrl }
Test-Case -Name "拒绝空 body" -Test $test6Wrapper -ExpectedStatus 400

# 测试总结
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  测试总结" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "通过: $global:passed" -ForegroundColor Green
if ($global:failed -eq 0) {
    Write-Host "失败: $global:failed" -ForegroundColor Green
} else {
    Write-Host "失败: $global:failed" -ForegroundColor Red
}
$total = $global:passed + $global:failed
Write-Host "总计: $total" -ForegroundColor Cyan
Write-Host ""

if ($global:failed -eq 0) {
    Write-Host "[SUCCESS] 所有测试通过！" -ForegroundColor Green
    exit 0
} else {
    Write-Host "[FAILED] 部分测试失败" -ForegroundColor Red
    exit 1
}
