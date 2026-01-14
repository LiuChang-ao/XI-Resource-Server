# 修复测试问题指南

## 问题描述

运行测试时遇到两个主要问题：

1. **go.sum 缺失条目**：缺少一些依赖的实际 hash 值
2. **SQLite CGO 问题**：测试需要 CGO 支持，但默认可能未启用

## 解决方案

### 方法1：自动修复（推荐）

运行更新后的构建脚本，它会自动处理依赖和 CGO：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 test
```

脚本已更新，会自动：
- 运行 `go mod tidy` 更新依赖
- 设置 `CGO_ENABLED=1` 以支持 SQLite 测试

### 方法2：手动修复

如果自动修复失败，可以手动执行以下步骤：

#### 步骤1：更新依赖

```powershell
# 在项目根目录
cd cloud
go mod tidy

cd ..\proto
go mod tidy

cd ..
```

#### 步骤2：运行测试（启用 CGO）

```powershell
# 测试 cloud 模块
cd cloud
$env:CGO_ENABLED = "1"
go test ./...

# 测试 agent 模块
cd ..\agent
$env:CGO_ENABLED = "1"
go test ./...
```

### 方法3：如果网络问题导致 go mod tidy 失败

如果遇到网络连接问题（无法访问 proxy.golang.org），可以：

1. **设置 Go 代理**（使用国内镜像）：
```powershell
$env:GOPROXY = "https://goproxy.cn,direct"
go mod tidy
```

2. **或者禁用校验和数据库**（仅用于开发环境）：
```powershell
$env:GOSUMDB = "off"
go mod tidy
```

## 验证修复

修复后，运行测试应该看到：

```
ok      github.com/xiresource/cloud/internal/api        X.XXXs
ok      github.com/xiresource/cloud/internal/job        X.XXXs
ok      github.com/xiresource/cloud/internal/gateway    X.XXXs
...
```

## 注意事项

- **CGO 要求**：SQLite 测试需要 CGO，确保系统已安装 C 编译器
  - Windows: 需要安装 MinGW 或 TDM-GCC
  - 如果无法安装 CGO，可以考虑使用纯 Go 的 SQLite 替代方案（如 `modernc.org/sqlite`）

- **go.sum 文件**：应该提交到版本控制中，确保团队所有成员使用相同的依赖版本
