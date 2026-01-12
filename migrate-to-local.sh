#!/bin/bash
# 迁移到本地模块路径脚本
# 将 github.com/xiresource/* 改为 local/xiresource/*

set -e

echo "=== 迁移到本地模块路径 ==="

# 备份提示
echo "注意：此脚本将修改多个文件，建议先提交当前更改或创建备份"
read -p "继续？(y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
fi

# 1. 修改 proto/go.mod
echo "1. 修改 proto/go.mod..."
sed -i 's|module github.com/xiresource/proto|module local/xiresource/proto|g' proto/go.mod
echo "✓ proto/go.mod 已更新"

# 2. 修改 proto/control.proto
echo "2. 修改 proto/control.proto..."
sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' proto/control.proto
echo "✓ proto/control.proto 已更新"

# 3. 重新生成 protobuf 代码
echo "3. 重新生成 protobuf 代码..."
cd proto
protoc --go_out=. --go_opt=module=local/xiresource/proto control.proto
go mod tidy
cd ..
echo "✓ protobuf 代码已重新生成"

# 4. 修改 cloud/go.mod
echo "4. 修改 cloud/go.mod..."
sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' cloud/go.mod
sed -i 's|replace github.com/xiresource/proto|replace local/xiresource/proto|g' cloud/go.mod
echo "✓ cloud/go.mod 已更新"

# 5. 修改 agent/go.mod
echo "5. 修改 agent/go.mod..."
sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' agent/go.mod
sed -i 's|replace github.com/xiresource/proto|replace local/xiresource/proto|g' agent/go.mod
echo "✓ agent/go.mod 已更新"

# 6. 修改所有 Go 文件中的 import
echo "6. 修改 Go 文件中的 import 语句..."
find cloud agent -name "*.go" -type f -exec sed -i 's|github.com/xiresource/proto/control|local/xiresource/proto/control|g' {} +
echo "✓ Go 文件已更新"

# 7. 更新依赖
echo "7. 更新模块依赖..."
cd cloud
go mod tidy
cd ..

cd agent
go mod tidy
cd ..

echo "✓ 依赖已更新"

# 8. 验证编译
echo "8. 验证编译..."
cd cloud
if go build -o ../bin/server-test ./cmd/server 2>/dev/null; then
    rm -f ../bin/server-test
    echo "✓ cloud 编译成功"
else
    echo "✗ cloud 编译失败，请检查错误"
    exit 1
fi
cd ..

echo ""
echo "=== 迁移完成！ ==="
echo "所有模块路径已从 github.com/xiresource/* 改为 local/xiresource/*"
echo "现在不需要设置 GOPRIVATE 了"
