# 故障排查指南 (Linux)

## protoc-gen-go 找不到问题

### 错误信息
```
protoc-gen-go: program not found or is not executable
Please specify a program using absolute path or make sure the program is available in your PATH system variable
--go_out: protoc-gen-go: Plugin failed with status code 1.
```

### 解决方案

#### 步骤 1: 安装 protoc-gen-go 插件

```bash
# 安装 Go protobuf 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

#### 步骤 2: 检查 Go 环境变量

```bash
# 检查 Go 版本和 GOPATH
go version
go env GOPATH

# 检查默认 Go bin 路径
echo $HOME/go/bin
```

#### 步骤 3: 将 Go bin 目录添加到 PATH

```bash
# 临时添加到 PATH (当前会话有效)
export PATH=$PATH:$(go env GOPATH)/bin
# 或者使用默认路径
export PATH=$PATH:$HOME/go/bin

# 永久添加到 PATH (推荐)
# 编辑 ~/.bashrc (或 ~/.zshrc)
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc

# 重新加载配置
source ~/.bashrc
# 或者重新登录/打开新终端
```

#### 步骤 4: 验证安装

```bash
# 验证 protoc-gen-go 是否在 PATH 中
which protoc-gen-go

# 检查版本
protoc-gen-go --version

# 如果仍然找不到，使用完整路径
ls -la $(go env GOPATH)/bin/protoc-gen-go
# 或
ls -la $HOME/go/bin/protoc-gen-go
```

#### 步骤 5: 重新生成 protobuf 代码

```bash
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
cd ..
```

### 常见问题

#### 问题 1: GOPATH 未设置

```bash
# 检查 GOPATH
go env GOPATH

# 如果为空，Go 1.11+ 使用模块模式，默认路径为 $HOME/go
# 确保 $HOME/go/bin 在 PATH 中
export PATH=$PATH:$HOME/go/bin
```

#### 问题 2: 权限问题

```bash
# 检查文件权限
ls -la $(go env GOPATH)/bin/protoc-gen-go

# 如果需要，添加执行权限
chmod +x $(go env GOPATH)/bin/protoc-gen-go
```

#### 问题 3: 网络问题 (无法下载插件)

```bash
# 如果在中国大陆，可以使用 Go 代理
export GOPROXY=https://goproxy.cn,direct

# 然后重新安装
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

#### 问题 4: conda 环境干扰

如果您使用 conda (如 base 环境)，可能需要：

```bash
# 确保使用的是系统的 Go，而不是 conda 的 Go
which go

# 如果需要，退出 conda 环境或使用系统的 Go
conda deactivate

# 或者确保 conda 环境中有正确的 Go
conda install -c conda-forge go
```

### 完整安装流程 (一次性脚本)

```bash
#!/bin/bash
set -e

echo "=== 安装 protoc-gen-go ==="

# 1. 检查 Go 是否安装
if ! command -v go &> /dev/null; then
    echo "错误: 未找到 Go，请先安装 Go 1.21 或更高版本"
    exit 1
fi

echo "Go 版本: $(go version)"

# 2. 设置 Go 代理 (可选，适用于中国大陆)
export GOPROXY=https://goproxy.cn,direct

# 3. 安装 protoc-gen-go
echo "正在安装 protoc-gen-go..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# 4. 获取 Go bin 路径
GO_BIN=$(go env GOPATH)/bin
if [ -z "$(go env GOPATH)" ]; then
    GO_BIN="$HOME/go/bin"
fi

# 5. 添加到 PATH
if [[ ":$PATH:" != *":$GO_BIN:"* ]]; then
    echo "添加 $GO_BIN 到 PATH..."
    echo "export PATH=\$PATH:$GO_BIN" >> ~/.bashrc
    export PATH=$PATH:$GO_BIN
    echo "已添加，请运行 'source ~/.bashrc' 或重新打开终端"
fi

# 6. 验证安装
if command -v protoc-gen-go &> /dev/null; then
    echo "✓ protoc-gen-go 安装成功!"
    protoc-gen-go --version
else
    echo "✗ protoc-gen-go 未找到，请检查 PATH"
    echo "尝试运行: export PATH=\$PATH:$GO_BIN"
    exit 1
fi

# 7. 测试生成 protobuf 代码
echo ""
echo "=== 测试生成 protobuf 代码 ==="
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
echo "✓ protobuf 代码生成成功!"
cd ..
```

保存为 `install-protoc-gen-go.sh`，然后运行：

```bash
chmod +x install-protoc-gen-go.sh
./install-protoc-gen-go.sh
```

## Go 模块依赖问题

### 错误信息
```
missing go.sum entry for module providing package github.com/gorilla/websocket
no required module provides package github.com/xiresource/proto/control
```

### 解决方案

#### 步骤 1: 确保 protobuf 代码已生成

```bash
# 检查 proto 代码是否存在
ls -la proto/control/control.pb.go

# 如果不存在，先生成
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
cd ..
```

#### 步骤 2: 更新 proto 模块依赖

```bash
cd proto
go mod tidy
cd ..
```

#### 步骤 3: 更新 cloud 模块依赖

```bash
cd cloud
go mod tidy
cd ..
```

#### 步骤 4: 重新编译

```bash
cd cloud
go build -o ../bin/server-linux-amd64 ./cmd/server
cd ..
```

### 修复 replace 指令问题

如果遇到 `module github.com/xiresource/proto/control@latest found, but does not contain package` 错误，需要修复 `go.mod` 中的 `replace` 指令：

**错误的 replace 指令：**
```go
replace github.com/xiresource/proto/control => ../proto
```

**正确的 replace 指令：**
```go
replace github.com/xiresource/proto => ../proto
```

修复方法：
```bash
# 编辑 cloud/go.mod，将 replace 指令改为：
# replace github.com/xiresource/proto => ../proto

# 编辑 agent/go.mod，同样修改 replace 指令

# 然后运行 go mod tidy
cd cloud && go mod tidy && cd ..
cd agent && go mod tidy && cd ..
```

### 完整修复脚本

```bash
#!/bin/bash
set -e

echo "=== 修复 Go 模块依赖 ==="

# 1. 生成 protobuf 代码
echo "生成 protobuf 代码..."
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
if [ ! -f "control/control.pb.go" ]; then
    echo "错误: protobuf 代码生成失败"
    exit 1
fi
echo "✓ protobuf 代码生成成功"

# 2. 更新 proto 模块依赖
echo "更新 proto 模块依赖..."
go mod tidy
cd ..

# 3. 修复 replace 指令（如果需要）
echo "检查 replace 指令..."
if grep -q "replace github.com/xiresource/proto/control =>" cloud/go.mod; then
    echo "修复 cloud/go.mod 中的 replace 指令..."
    sed -i 's|replace github.com/xiresource/proto/control =>|replace github.com/xiresource/proto =>|g' cloud/go.mod
fi
if grep -q "replace github.com/xiresource/proto/control =>" agent/go.mod; then
    echo "修复 agent/go.mod 中的 replace 指令..."
    sed -i 's|replace github.com/xiresource/proto/control =>|replace github.com/xiresource/proto =>|g' agent/go.mod
fi

# 4. 更新 cloud 模块依赖
echo "更新 cloud 模块依赖..."
cd cloud
go mod tidy
cd ..

# 5. 更新 agent 模块依赖
echo "更新 agent 模块依赖..."
cd agent
go mod tidy
cd ..

# 6. 验证依赖
echo "验证依赖..."
cd cloud
go mod verify
cd ..

echo "✓ 所有依赖已更新"
```

### 网络问题 (中国大陆)

如果下载依赖较慢，可以使用 Go 代理：

```bash
# 设置 Go 代理
export GOPROXY=https://goproxy.cn,direct

# 然后运行 go mod tidy
cd proto && go mod tidy && cd ..
cd cloud && go mod tidy && cd ..
```

## 其他常见问题

### protoc 未安装

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y protobuf-compiler

# CentOS/RHEL
sudo yum install protobuf-compiler

# 验证
protoc --version
```

### Go 版本过低

```bash
# 检查 Go 版本 (需要 1.21+)
go version

# 如果需要升级，访问 https://go.dev/dl/
# 下载并安装新版本
```
