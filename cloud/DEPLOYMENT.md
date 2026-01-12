# Cloud Server 部署指南 (Linux)

本文档说明如何在 Linux 环境下打包 cloud server 并以二进制方式运行。

## 0. 前置依赖安装

### 0.1 安装 Go (如果未安装)

```bash
# 检查 Go 版本
go version

# 如果未安装，请先安装 Go 1.21 或更高版本
# Ubuntu/Debian:
# wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
# sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
# echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
# source ~/.bashrc
```

### 0.2 安装 Protocol Buffers 编译器

```bash
# Ubuntu/Debian:
sudo apt-get update
sudo apt-get install -y protobuf-compiler

# CentOS/RHEL:
# sudo yum install protobuf-compiler

# 验证安装
protoc --version
```

### 0.3 安装 protoc-gen-go 插件 (必需)

```bash
# 安装 Go protobuf 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# 确保 Go 的 bin 目录在 PATH 中
# 检查 Go 环境变量
go env GOPATH
# 或者使用默认路径
echo $HOME/go/bin

# 将 $HOME/go/bin 添加到 PATH (如果不在 PATH 中)
export PATH=$PATH:$(go env GOPATH)/bin
# 或者使用默认路径
export PATH=$PATH:$HOME/go/bin

# 永久添加到 PATH (添加到 ~/.bashrc 或 ~/.zshrc)
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc

# 验证 protoc-gen-go 是否安装成功
protoc-gen-go --version
# 或者
which protoc-gen-go
```

**常见问题：**
- 如果仍然提示 `protoc-gen-go: program not found`，请确保：
  1. `go install` 命令成功执行（没有错误）
  2. `$HOME/go/bin` 或 `$(go env GOPATH)/bin` 在 PATH 中
  3. 重新加载 shell 配置：`source ~/.bashrc` 或重新打开终端

### 0.4 验证所有依赖

```bash
# 检查所有必需的依赖
echo "Go version:"
go version

echo "protoc version:"
protoc --version

echo "protoc-gen-go location:"
which protoc-gen-go
protoc-gen-go --version
```

## 1. 编译打包

### 在 Linux 环境下编译

```bash
# 确保在项目根目录
cd ~/XI-Resource-Server

# 步骤 1: 生成 protobuf 代码
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
cd ..

# 步骤 2: 更新 proto 模块依赖
cd proto
go mod tidy
cd ..

# 步骤 3: 验证 cloud/go.mod 中的 replace 指令
# 确保 cloud/go.mod 包含: replace github.com/xiresource/proto => ../proto
grep "replace github.com/xiresource/proto" cloud/go.mod

# 步骤 4: 更新 cloud 模块依赖（从项目根目录执行，确保相对路径正确）
cd cloud
go mod tidy
cd ..

# 步骤 5: 验证依赖是否正确解析（可选）
cd cloud
go list -m github.com/xiresource/proto/control
# 应该显示: github.com/xiresource/proto/control (replaced by ../proto)
cd ..

# 步骤 6: 编译 cloud server (生成二进制文件)
cd cloud
go build -o ../bin/server-linux-amd64 ./cmd/server
cd ..
```
   ```
### 跨平台编译 (在其他系统上编译 Linux 版本)

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o bin/server-linux-amd64 ./cloud/cmd/server

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o bin/server-linux-arm64 ./cloud/cmd/server
```

## 2. 配置方式

Cloud server 支持以下配置方式（优先级从高到低）：
1. 命令行参数
2. 环境变量
3. `.env` 配置文件
4. 默认值

### 2.1 命令行参数

```bash
./bin/server-linux-amd64 \
  -addr :8080 \              # HTTP 服务器监听地址 (默认: :8080)
  -wss-path /wss \           # WebSocket 路径 (默认: /wss)
  -dev false \               # 开发模式，禁用 TLS (默认: false)
  -db jobs.db \              # SQLite 数据库路径 (默认: jobs.db, 仅当未配置 MySQL 时生效)
  -env .env                  # .env 配置文件路径 (可选, 会自动搜索)
```

### 2.2 环境变量配置

#### 数据库配置 (MySQL 或 SQLite)

**MySQL 配置:**
```bash
export DB_TYPE=mysql
export MYSQL_HOST=localhost
export MYSQL_PORT=3306
export MYSQL_USER=your_user
export MYSQL_PASSWORD=your_password
export MYSQL_DATABASE=jobs_db
export MYSQL_PARAMS="charset=utf8mb4&parseTime=True&loc=Local"  # 可选
```

**SQLite 配置 (默认):**
```bash
export DB_TYPE=sqlite
export SQLITE_PATH=/var/lib/xiresource/jobs.db  # 可选, 默认: jobs.db
```

#### Redis 配置 (可选, 但推荐用于任务调度)

**方式 1: Redis URL (推荐)**
```bash
export REDIS_URL=redis://localhost:6379/0
# 带认证: redis://username:password@localhost:6379/0
# TLS: rediss://username:password@localhost:6379/0
```

**方式 2: 独立配置**
```bash
export REDIS_HOST=localhost
export REDIS_PORT=6379
export REDIS_PASSWORD=your_password    # 可选
export REDIS_DATABASE=0                # 可选, 默认 0
export REDIS_USERNAME=your_username    # 可选
export REDIS_TLS_ENABLED=false         # 可选
```

#### OSS 配置 (必需, 用于任务分配)

```bash
export COS_SECRET_ID=your_secret_id
export COS_SECRET_KEY=your_secret_key
export COS_BUCKET=your_bucket_name
export COS_REGION=ap-beijing
export COS_PRESIGN_TTL_MINUTES=15      # 可选, 默认 15 分钟
export COS_BASE_URL=                   # 可选, 自动生成
```

### 2.3 .env 配置文件

在项目根目录或 `cloud/` 目录下创建 `.env` 文件：

```env
# 数据库配置 (MySQL)
DB_TYPE=mysql
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=your_user
MYSQL_PASSWORD=your_password
MYSQL_DATABASE=jobs_db
MYSQL_PARAMS=charset=utf8mb4&parseTime=True&loc=Local

# 或使用 SQLite (默认)
# DB_TYPE=sqlite
# SQLITE_PATH=/var/lib/xiresource/jobs.db

# Redis 配置 (推荐)
REDIS_URL=redis://localhost:6379/0

# OSS 配置 (必需)
COS_SECRET_ID=your_secret_id
COS_SECRET_KEY=your_secret_key
COS_BUCKET=your_bucket_name
COS_REGION=ap-beijing
COS_PRESIGN_TTL_MINUTES=15
```

## 3. 运行示例

### 3.1 使用 .env 文件 (推荐)

```bash
# 创建 .env 文件
cat > .env << 'EOF'
DB_TYPE=sqlite
SQLITE_PATH=/var/lib/xiresource/jobs.db
REDIS_URL=redis://localhost:6379/0
COS_SECRET_ID=your_secret_id
COS_SECRET_KEY=your_secret_key
COS_BUCKET=your_bucket_name
COS_REGION=ap-beijing
EOF

# 运行 server (自动加载 .env)
./bin/server-linux-amd64 -addr :8080
```

### 3.2 使用环境变量

```bash
export DB_TYPE=sqlite
export SQLITE_PATH=/var/lib/xiresource/jobs.db
export REDIS_URL=redis://localhost:6379/0
export COS_SECRET_ID=your_secret_id
export COS_SECRET_KEY=your_secret_key
export COS_BUCKET=your_bucket_name
export COS_REGION=ap-beijing

./bin/server-linux-amd64 -addr :8080
```

### 3.3 使用命令行参数指定配置文件

```bash
./bin/server-linux-amd64 \
  -addr :8080 \
  -env /etc/xiresource/.env
```

### 3.4 生产环境示例

```bash
# 使用 systemd 服务文件 (见下文)
sudo systemctl start xiresource-cloud

# 或直接运行
sudo -u xiresource \
  DB_TYPE=mysql \
  MYSQL_HOST=db.example.com \
  MYSQL_PORT=3306 \
  MYSQL_USER=xiresource \
  MYSQL_PASSWORD='secure_password' \
  MYSQL_DATABASE=xiresource_jobs \
  REDIS_URL=redis://redis.example.com:6379/0 \
  COS_SECRET_ID=your_secret_id \
  COS_SECRET_KEY=your_secret_key \
  COS_BUCKET=your_bucket_name \
  COS_REGION=ap-beijing \
  ./bin/server-linux-amd64 \
    -addr :8080
```

## 4. 监听端口配置

### 4.1 默认端口

默认监听端口为 `:8080`，表示监听所有网络接口的 8080 端口。

### 4.2 自定义端口

```bash
# 监听 443 端口 (生产环境推荐用于 TLS)
./bin/server-linux-amd64 -addr :443

# 监听 8443 端口
./bin/server-linux-amd64 -addr :8443

# 仅监听本地回环地址 (127.0.0.1:8080)
./bin/server-linux-amd64 -addr 127.0.0.1:8080

# 监听特定 IP 地址
./bin/server-linux-amd64 -addr 192.168.1.100:8080
```

### 4.3 使用端口号小于 1024 (需要 root 权限)

```bash
# 使用 sudo 运行 (不推荐, 应使用反向代理)
sudo ./bin/server-linux-amd64 -addr :443

# 更好的方式: 使用反向代理 (nginx/caddy) 监听 443, server 监听 8080
```

## 5. Systemd 服务配置

创建 systemd 服务文件以便管理：

```bash
sudo nano /etc/systemd/system/xiresource-cloud.service
```

```ini
[Unit]
Description=XI Resource Cloud Server
After=network.target mysql.service redis.service

[Service]
Type=simple
User=xiresource
Group=xiresource
WorkingDirectory=/opt/xiresource/cloud
ExecStart=/opt/xiresource/cloud/bin/server-linux-amd64 -addr :8080 -env /etc/xiresource/.env
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# 环境变量 (可选, 也可以放在 .env 文件中)
Environment="DB_TYPE=mysql"
Environment="MYSQL_HOST=localhost"
Environment="MYSQL_PORT=3306"
# ... 其他环境变量

[Install]
WantedBy=multi-user.target
```

启用并启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable xiresource-cloud
sudo systemctl start xiresource-cloud
sudo systemctl status xiresource-cloud
```

查看日志：

```bash
sudo journalctl -u xiresource-cloud -f
```

## 6. 使用反向代理 (推荐生产环境)

### 6.1 Nginx 配置示例

```nginx
upstream xiresource_cloud {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # WebSocket 支持
    location /wss {
        proxy_pass http://xiresource_cloud;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
    }

    # HTTP API
    location / {
        proxy_pass http://xiresource_cloud;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 6.2 Server 配置

Server 监听本地 8080 端口：

```bash
./bin/server-linux-amd64 -addr 127.0.0.1:8080
```

## 7. 健康检查

启动后，可以通过以下端点检查服务状态：

```bash
# 健康检查
curl http://localhost:8080/health

# 查看在线 Agent
curl http://localhost:8080/api/agents/online

# 查看任务列表
curl http://localhost:8080/api/jobs
```

## 8. 完整部署检查清单

- [ ] 编译生成二进制文件
- [ ] 创建 `.env` 配置文件或设置环境变量
- [ ] 配置数据库 (MySQL 或 SQLite)
- [ ] 配置 Redis (推荐)
- [ ] 配置 OSS (必需)
- [ ] 设置监听端口 (默认 :8080)
- [ ] 配置防火墙规则 (如果直接暴露端口)
- [ ] 配置反向代理 (生产环境推荐)
- [ ] 配置 TLS 证书 (生产环境必需, 不使用 `-dev` 模式)
- [ ] 创建 systemd 服务 (可选但推荐)
- [ ] 测试健康检查端点
- [ ] 测试 WebSocket 连接 (`wss://your-domain.com/wss`)

## 9. 故障排查

### 端口已被占用

```bash
# 检查端口占用
sudo netstat -tlnp | grep :8080
# 或
sudo ss -tlnp | grep :8080

# 杀死占用进程
sudo kill -9 <PID>
```

### 权限问题

```bash
# 如果使用小于 1024 的端口, 需要 root 权限
# 或使用 setcap (推荐)
sudo setcap 'cap_net_bind_service=+ep' /opt/xiresource/cloud/bin/server-linux-amd64
```

### 配置文件未加载

- 检查 `.env` 文件路径是否正确
- 使用 `-env` 参数显式指定配置文件路径
- 检查文件权限 (服务用户是否有读取权限)

### 数据库连接失败

- 检查数据库服务是否运行
- 验证连接参数 (主机、端口、用户名、密码、数据库名)
- 检查防火墙规则
- 查看 server 日志中的错误信息

## 10. 安全建议

1. **不要在生产环境使用 `-dev` 模式** (禁用 TLS)
2. **使用反向代理处理 TLS**，server 监听本地端口
3. **保护 `.env` 文件权限**：`chmod 600 .env`
4. **使用非 root 用户运行** server
5. **定期更新** server 二进制文件
6. **监控日志** 检查异常行为
7. **配置防火墙** 限制访问来源
8. **使用强密码** 保护数据库和 Redis
