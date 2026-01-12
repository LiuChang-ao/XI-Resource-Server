# Windows 上安装 CGO 支持

CGO 需要 C 编译器。在 Windows 上有几种安装方式：

## 方法 1: 使用 MSYS2 (推荐)

MSYS2 提供了完整的开发环境，包括 GCC。

### 安装步骤：

1. **下载并安装 MSYS2**
   - 访问 https://www.msys2.org/
   - 下载并运行安装程序
   - 安装到默认路径 `C:\msys64`

2. **安装 GCC 工具链**
   ```powershell
   # 在 MSYS2 终端中运行（不是 PowerShell）
   pacman -Syu
   pacman -S mingw-w64-x86_64-gcc
   ```

3. **添加到 PATH**
   - 将 `C:\msys64\mingw64\bin` 添加到系统 PATH
   - 或者在 PowerShell 中临时设置：
   ```powershell
   $env:PATH = "C:\msys64\mingw64\bin;$env:PATH"
   ```

4. **验证安装**
   ```powershell
   gcc --version
   ```

## 方法 2: 使用 TDM-GCC (简单快速)

TDM-GCC 是预编译的 MinGW-w64 发行版。

### 安装步骤：

1. **下载 TDM-GCC**
   - 访问 https://jmeubank.github.io/tdm-gcc/
   - 下载 TDM-GCC 64-bit 安装程序

2. **安装**
   - 运行安装程序
   - 选择 "Add to PATH" 选项

3. **验证**
   ```powershell
   gcc --version
   ```

## 方法 3: 使用 Chocolatey (如果已安装)

```powershell
# 以管理员身份运行 PowerShell
choco install mingw
```

## 启用 CGO

安装 C 编译器后，设置环境变量：

```powershell
# 临时启用（当前会话）
$env:CGO_ENABLED = "1"

# 永久启用（添加到用户环境变量）
[System.Environment]::SetEnvironmentVariable("CGO_ENABLED", "1", "User")
```

## 验证 CGO 是否工作

```powershell
# 检查 Go 环境
go env CGO_ENABLED

# 运行测试
cd cloud
go test ./internal/job -v
```

## 常见问题

### 问题：`gcc: command not found`
- 确保 GCC 已添加到 PATH
- 重启 PowerShell 或重新加载环境变量

### 问题：`CGO_ENABLED=0`
- 检查环境变量：`$env:CGO_ENABLED`
- 如果为空，设置为 "1"

### 问题：链接错误
- 确保安装了完整的 MinGW-w64 工具链
- 不要只安装 GCC，还需要 binutils

## 快速测试脚本

创建 `test-cgo.ps1`：

```powershell
# 检查 GCC
Write-Host "Checking GCC..."
gcc --version
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: GCC not found. Please install a C compiler."
    exit 1
}

# 检查 CGO
Write-Host "`nChecking CGO..."
$cgo = go env CGO_ENABLED
Write-Host "CGO_ENABLED = $cgo"
if ($cgo -ne "1") {
    Write-Host "WARNING: CGO is disabled. Setting CGO_ENABLED=1..."
    $env:CGO_ENABLED = "1"
}

# 运行测试
Write-Host "`nRunning tests..."
cd cloud
go test ./internal/job -v
```
