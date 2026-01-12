# 迁移到本地模块路径

将 `github.com/xiresource/*` 改为本地模块路径，避免 Go 尝试从 GitHub 下载。

## 方案：使用 `local/xiresource/*` 路径

### 步骤 1: 修改 proto 模块

```bash
# 1. 修改 proto/go.mod
cd proto
# 将 module github.com/xiresource/proto 改为：
# module local/xiresource/proto
```

### 步骤 2: 修改 proto/control.proto

```proto
// 将 go_package 改为：
option go_package = "local/xiresource/proto/control;control";
```

### 步骤 3: 重新生成 protobuf 代码

```bash
cd proto
protoc --go_out=. --go_opt=module=local/xiresource/proto control.proto
go mod tidy
```

### 步骤 4: 修改 cloud 和 agent 模块

```bash
# 修改 cloud/go.mod
cd cloud
# 1. 修改 module 名称（可选，但建议保持一致）
# module local/xiresource/cloud

# 2. 修改 require 和 replace
# require local/xiresource/proto/control v0.0.0
# replace local/xiresource/proto => ../proto

# 3. 修改所有 import 语句
# 将 github.com/xiresource/proto/control 改为 local/xiresource/proto/control
```

### 步骤 5: 修改所有代码中的 import

需要修改的文件：
- `cloud/internal/gateway/gateway.go`
- `cloud/internal/gateway/gateway_test.go`
- `agent/internal/client/client.go`
- `agent/internal/client/client_test.go`

将：
```go
import control "github.com/xiresource/proto/control"
```

改为：
```go
import control "local/xiresource/proto/control"
```

## 自动化迁移脚本

可以使用以下脚本自动完成迁移：

```bash
#!/bin/bash
# migrate-to-local.sh

# 1. 修改 proto/go.mod
sed -i 's|module github.com/xiresource/proto|module local/xiresource/proto|g' proto/go.mod

# 2. 修改 proto/control.proto
sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' proto/control.proto

# 3. 重新生成 protobuf
cd proto
protoc --go_out=. --go_opt=module=local/xiresource/proto control.proto
go mod tidy
cd ..

# 4. 修改 cloud/go.mod
sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' cloud/go.mod
sed -i 's|replace github.com/xiresource/proto|replace local/xiresource/proto|g' cloud/go.mod

# 5. 修改 agent/go.mod
sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' agent/go.mod
sed -i 's|replace github.com/xiresource/proto|replace local/xiresource/proto|g' agent/go.mod

# 6. 修改所有 Go 文件中的 import
find cloud agent -name "*.go" -type f -exec sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' {} +

# 7. 更新依赖
cd cloud && go mod tidy && cd ..
cd agent && go mod tidy && cd ..
```

## 其他可选路径

如果不想使用 `local/xiresource/*`，也可以使用：

1. **`xiresource/proto`** - 更简洁
2. **`internal/proto`** - 如果是内部使用（但 `internal` 在 Go 中有特殊含义）
3. **`company/proto`** - 使用公司名
4. **`project/proto`** - 使用项目名

## 注意事项

1. **模块路径一旦更改，需要更新所有引用**
2. **protobuf 代码需要重新生成**
3. **如果将来要发布到真实的仓库，需要再次修改**
4. **使用 `local/` 前缀可以明确表示这是本地模块**

## 优势

- ✅ 不需要设置 GOPRIVATE
- ✅ 不会尝试从远程下载
- ✅ 更明确这是本地模块
- ✅ 避免网络相关问题
