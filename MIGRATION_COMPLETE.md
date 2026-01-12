# 模块路径迁移完成

已将所有模块路径从 `github.com/xiresource/*` 迁移到 `xiresource.local/*`。

## 修改内容

### 1. Proto 模块
- `proto/go.mod`: `module xiresource.local/proto`
- `proto/control.proto`: `option go_package = "xiresource.local/proto/control;control"`

### 2. Cloud 模块
- `cloud/go.mod`: 
  - require: `xiresource.local/proto/control v0.0.0`
  - replace: `xiresource.local/proto => ../proto`
- `cloud/internal/gateway/gateway.go`: import 已更新
- `cloud/internal/gateway/gateway_test.go`: import 已更新

### 3. Agent 模块
- `agent/go.mod`:
  - require: `xiresource.local/proto/control v0.0.0`
  - replace: `xiresource.local/proto => ../proto`
- `agent/internal/client/client.go`: import 已更新
- `agent/internal/client/client_test.go`: import 已更新

### 4. 构建脚本
- `Makefile`: protoc 命令已更新
- `scripts/build.ps1`: protoc 命令已更新

## 下一步操作

### 1. 重新生成 Protobuf 代码

**Windows (PowerShell):**
```powershell
cd proto
protoc --go_out=. --go_opt=module=xiresource.local/proto control.proto
```

**Linux/macOS:**
```bash
cd proto
protoc --go_out=. --go_opt=module=xiresource.local/proto control.proto
```

### 2. 更新依赖

```bash
# Proto 模块
cd proto
go mod tidy

# Cloud 模块
cd ../cloud
go mod tidy

# Agent 模块
cd ../agent
go mod tidy
```

### 3. 编译验证

**Windows:**
```powershell
# 编译 cloud server
cd cloud
go build -o ..\bin\server.exe .\cmd\server

# 编译 agent
cd ..\agent
go build -o ..\bin\agent.exe .\cmd\agent
```

**Linux/macOS:**
```bash
# 编译 cloud server
cd cloud
go build -o ../bin/server ./cmd/server

# 编译 agent
cd ../agent
go build -o ../bin/agent ./cmd/agent
```

## 优势

✅ **不再需要设置 GOPRIVATE** - `xiresource.local` 是本地域名格式，Go 不会尝试从远程下载  
✅ **避免网络问题** - 所有依赖都通过 `replace` 指令使用本地路径  
✅ **符合 Go 规范** - 使用 `.local` 域名格式，符合 Go 模块路径要求  
✅ **更清晰的意图** - 明确表示这是本地模块，不会发布到远程仓库  

## 注意事项

1. **protoc-gen-go 需要安装** - 如果遇到 `protoc-gen-go: program not found` 错误，需要安装：
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   ```

2. **网络代理** - 如果下载其他依赖（如 `google.golang.org/protobuf`）时遇到网络问题，可以设置 Go 代理：
   ```bash
   # Windows PowerShell
   $env:GOPROXY = "https://goproxy.cn,direct"
   
   # Linux/macOS
   export GOPROXY=https://goproxy.cn,direct
   ```

3. **清理旧缓存** - 如果之前使用过 `github.com/xiresource/*` 路径，可以清理缓存：
   ```bash
   go clean -modcache
   ```

## 验证

编译成功后，可以验证二进制文件：

```bash
# Windows
.\bin\server.exe --help

# Linux/macOS
./bin/server --help
```

## 回滚（如果需要）

如果将来需要回滚到 `github.com/xiresource/*` 路径，可以使用 Git 恢复：

```bash
git checkout HEAD -- proto/go.mod proto/control.proto cloud/go.mod agent/go.mod
git checkout HEAD -- cloud/internal/gateway/*.go agent/internal/client/*.go
git checkout HEAD -- Makefile scripts/build.ps1
```

然后重新生成 protobuf 代码并更新依赖。
