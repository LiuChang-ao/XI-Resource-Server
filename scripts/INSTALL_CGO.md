# 快速安装 CGO 支持 (Windows)

## 最简单方法：安装 TDM-GCC

### 步骤 1: 下载 TDM-GCC
访问：https://jmeubank.github.io/tdm-gcc/
- 下载 **TDM-GCC 64-bit** 安装程序（推荐最新版本）

### 步骤 2: 安装
1. 运行下载的安装程序
2. **重要**：在安装过程中，勾选 **"Add to PATH"** 选项
3. 完成安装

### 步骤 3: 验证安装
打开新的 PowerShell 窗口，运行：
```powershell
gcc --version
```
应该看到 GCC 版本信息。

### 步骤 4: 启用 CGO
```powershell
# 临时启用（当前会话）
$env:CGO_ENABLED = "1"

# 永久启用（推荐）
[System.Environment]::SetEnvironmentVariable("CGO_ENABLED", "1", "User")
```

### 步骤 5: 测试
```powershell
cd cloud
go test ./internal/job -v
```

---

## 替代方法：使用 Chocolatey（如果已安装）

```powershell
# 以管理员身份运行
choco install mingw
```

然后设置 CGO：
```powershell
$env:CGO_ENABLED = "1"
```

---

## 验证脚本

运行检查脚本：
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\setup-cgo.ps1
```
