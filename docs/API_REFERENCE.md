# Cloud Server HTTP API Reference

本文档描述了外部业务进程调用Cloud服务器的HTTP REST API接口。

## Base URL

- **开发环境**: `http://localhost:8080`
- **生产环境**: `https://your-domain.com` (使用TLS)

## 认证

当前MVP版本暂未实现认证机制。生产环境应添加API密钥或OAuth2认证。

## 通用响应格式

所有API响应使用JSON格式，Content-Type为`application/json`。

### 错误响应

```json
{
  "error": "Error message"
}
```

HTTP状态码：
- `200 OK` - 请求成功
- `201 Created` - 资源创建成功
- `400 Bad Request` - 请求参数错误
- `404 Not Found` - 资源不存在
- `405 Method Not Allowed` - HTTP方法不允许
- `413 Request Entity Too Large` - 请求体过大
- `415 Unsupported Media Type` - 不支持的Content-Type
- `500 Internal Server Error` - 服务器内部错误

---

## 端点列表

### 1. 健康检查

检查服务器健康状态。

**请求**
```
GET /health
```

**响应**
```json
{
  "status": "ok"
}
```

**状态码**: `200 OK`

---

### 2. 获取在线Agent列表

获取当前在线的所有Agent信息。

**请求**
```
GET /api/agents/online
```

**响应**
```json
[
  {
    "agent_id": "agent-001",
    "hostname": "WORKSTATION-01",
    "max_concurrency": 1,
    "paused": false,
    "running_jobs": 0,
    "last_heartbeat": "2026-01-12T10:30:45Z",
    "connected_at": "2026-01-12T10:00:00Z"
  }
]
```

**字段说明**:
- `agent_id`: Agent唯一标识符
- `hostname`: Agent所在主机名
- `max_concurrency`: 最大并发作业数
- `paused`: 是否暂停接受新作业
- `running_jobs`: 当前正在运行的作业数
- `last_heartbeat`: 最后心跳时间（ISO 8601格式）
- `connected_at`: 连接时间（ISO 8601格式）

**状态码**: `200 OK`

---

### 3. 创建作业

创建一个新的计算作业。

**请求**
```
POST /api/jobs
Content-Type: application/json
```

**请求体**
```json
{
  "input_bucket": "my-bucket",
  "input_key": "inputs/job-123/image.jpg",
  "output_bucket": "my-bucket",
  "output_key": "optional-specific-output-key",
  "output_prefix": "optional-output-prefix/",
  "output_extension": "json",
  "attempt_id": 1,
  "command": "python C:/scripts/analyze.py {input} {output}",
  "job_type": "COMMAND",
  "forward_url": "http://127.0.0.1:8080/api",
  "forward_method": "POST",
  "forward_headers": {
    "X-App-Token": "local-token"
  },
  "forward_body": "{\"mode\":\"fast\"}",
  "forward_timeout_sec": 60,
  "input_forward_mode": "URL"
}
```

**字段说明**:
- `input_bucket` (必需): OSS输入bucket名称
- `input_key` (必需): OSS输入对象key
- `output_bucket` (必需): OSS输出bucket名称
- `output_key` (可选): 特定输出key。如果未指定，将使用默认格式 `jobs/{job_id}/{attempt_id}/output.{extension}`
- `output_prefix` (可选): 输出前缀。如果未指定，将使用默认格式 `jobs/{job_id}/{attempt_id}/`
- `output_extension` (可选): 输出文件扩展名（不含点号），例如: `"json"`, `"txt"`, `"bin"`。默认为 `"bin"`
- `attempt_id` (可选): 作业尝试次数，默认为1
- `command` (可选): 在Agent上执行的命令。支持占位符：
  - `{input}`: 输入文件路径（Agent下载后）
  - `{output}`: 输出文件路径（Agent应写入此路径）
  - 示例: `"python C:/scripts/analyze.py {input} {output}"`
  - 最大长度: 8192字符
  - **注意**: 仅 `job_type=COMMAND` 时使用
- `job_type` (可选): 作业类型，默认 `COMMAND`。可选值：
  - `COMMAND`: 执行命令
  - `FORWARD_HTTP`: 转发请求到Agent所在机器的本地HTTP服务
- `forward_url` (可选): `FORWARD_HTTP` 时必填，本地服务URL
- `forward_method` (可选): `FORWARD_HTTP` 时使用的HTTP方法（默认 `POST`）
- `forward_headers` (可选): `FORWARD_HTTP` 时附加的HTTP请求头（透传给本地服务）
  - 示例中的 `X-App-Token` 仅为示例自定义头，可用于本地服务认证/鉴权
- `forward_body` (可选): `FORWARD_HTTP` 时的请求体（原样透传）
- `forward_timeout_sec` (可选): `FORWARD_HTTP` 时的请求超时（秒）
- `input_forward_mode` (可选): 输入文件转发方式（默认 `URL`）
  - `URL`: Agent不下载输入，只把presigned URL传给本地服务
  - `LOCAL_FILE`: Agent下载输入并以multipart上传给本地服务（字段名 `file`）

**安全限制**:
- 请求体大小限制: 1MB
- 仅接受 `application/json` Content-Type
- 拒绝 `multipart/form-data`（防止文件上传绕过）

**响应**
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "PENDING",
  "created_at": "2026-01-12T10:30:45Z"
}
```

**示例：转发到本地服务（URL模式）**
```json
{
  "input_bucket": "my-bucket",
  "input_key": "inputs/job-123/image.jpg",
  "output_bucket": "my-bucket",
  "output_extension": "json",
  "job_type": "FORWARD_HTTP",
  "forward_url": "http://127.0.0.1:8080/api/analyze",
  "forward_method": "POST",
  "input_forward_mode": "URL"
}
```

**示例：转发到本地服务（本地文件模式）**
```json
{
  "input_bucket": "my-bucket",
  "input_key": "inputs/job-123/image.jpg",
  "output_bucket": "my-bucket",
  "output_extension": "json",
  "job_type": "FORWARD_HTTP",
  "forward_url": "http://127.0.0.1:8080/api/analyze",
  "input_forward_mode": "LOCAL_FILE",
  "forward_headers": {
    "X-App-Token": "local-token"
  }
}
```

**状态码**: `201 Created`

**错误示例**:
```json
{
  "error": "input_bucket and input_key are required (OSS keys only, not file content)"
}
```

---

### 4. 获取作业详情

根据job_id获取作业的详细信息。

**请求**
```
GET /api/jobs/{job_id}
```

**路径参数**:
- `job_id`: 作业UUID

**响应**
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2026-01-12T10:30:45Z",
  "status": "SUCCEEDED",
  "input_bucket": "my-bucket",
  "input_key": "inputs/job-123/image.jpg",
  "output_bucket": "my-bucket",
  "output_key": "jobs/550e8400-e29b-41d4-a716-446655440000/1/output.json",
  "output_prefix": "jobs/550e8400-e29b-41d4-a716-446655440000/1/",
  "output_extension": "json",
  "attempt_id": 1,
  "assigned_agent_id": "agent-001",
  "lease_id": "lease-uuid",
  "lease_deadline": null,
  "command": "python C:/scripts/analyze.py {input} {output}",
  "job_type": "COMMAND",
  "forward_url": "",
  "forward_method": "",
  "forward_headers": "",
  "forward_body": "",
  "forward_timeout": 0,
  "input_forward_mode": "",
  "stdout": "Analysis completed. Output written to: C:\\...\\output.json",
  "stderr": ""
}
```

**字段说明**:
- `output_extension`: 输出文件扩展名（例如: `"json"`, `"txt"`, `"bin"`）
- `job_type`: 作业类型（`COMMAND`/`FORWARD_HTTP`）
- `forward_url`/`forward_method`/`forward_headers`/`forward_body`/`forward_timeout`: 转发作业配置
- `input_forward_mode`: 输入转发方式（`URL`/`LOCAL_FILE`）
- `stdout`: 命令执行的stdout输出（截断到10KB，如果为空则字段为空字符串）
- `stderr`: 命令执行的stderr输出（截断到10KB，通常在FAILED状态时包含错误信息）
- `output_key`: 如果命令没有产生输出文件（仅stdout），此字段可能为空字符串

**作业状态**:
- `PENDING`: 等待分配
- `ASSIGNED`: 已分配给Agent
- `RUNNING`: Agent正在执行
- `SUCCEEDED`: 执行成功
- `FAILED`: 执行失败
- `CANCELED`: 已取消
- `LOST`: 丢失（Agent断开或租约过期）

**状态码**: `200 OK`

**错误响应**:
- `400 Bad Request`: job_id格式无效
- `404 Not Found`: 作业不存在

---

### 5. 列出作业

获取作业列表，支持分页和状态过滤。

**请求**
```
GET /api/jobs?limit=100&offset=0&status=SUCCEEDED
```

**查询参数**:
- `limit` (可选): 返回的最大作业数，默认100，最大1000
- `offset` (可选): 分页偏移量，默认0
- `status` (可选): 状态过滤，可选值: `PENDING`, `ASSIGNED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELED`, `LOST`

**响应**
```json
[
  {
    "job_id": "550e8400-e29b-41d4-a716-446655440000",
    "created_at": "2026-01-12T10:30:45Z",
    "status": "SUCCEEDED",
    "input_bucket": "my-bucket",
    "input_key": "inputs/job-123/image.jpg",
    "output_bucket": "my-bucket",
    "output_key": "jobs/550e8400-e29b-41d4-a716-446655440000/1/output.json",
    "output_prefix": "jobs/550e8400-e29b-41d4-a716-446655440000/1/",
    "output_extension": "json",
    "attempt_id": 1,
    "assigned_agent_id": "agent-001",
    "lease_id": "lease-uuid",
    "lease_deadline": null,
    "command": "python C:/scripts/analyze.py {input} {output}",
    "stdout": "Analysis completed. Output written to: C:\\...\\output.json",
    "stderr": ""
  }
]
```

**状态码**: `200 OK`

---

## 使用示例

### 示例1: 创建图片分析作业

```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "my-bucket",
    "input_key": "uploads/user123/image.jpg",
    "output_bucket": "my-bucket",
    "output_extension": "json",
    "command": "python C:/scripts/analyze_image.py {input} {output}"
  }'
```

响应:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "PENDING",
  "created_at": "2026-01-12T10:30:45Z"
}
```

**Python脚本示例 (`analyze_image.py`)**:

Agent执行命令时，会将`{input}`和`{output}`替换为实际的文件路径。脚本应该以文件形式读取输入和写入输出：

```python
#!/usr/bin/env python3
"""
图片分析脚本示例
Agent会将 {input} 替换为输入文件路径，{output} 替换为输出文件路径
"""
import sys
import json
from PIL import Image  # 假设使用PIL进行图片处理

def analyze_image(input_path, output_path):
    """
    分析图片并输出结果
    
    Args:
        input_path: 输入图片文件路径（Agent已下载的文件）
        output_path: 输出结果文件路径（脚本应将结果写入此文件）
    """
    try:
        # 1. 从文件路径读取输入图片
        with open(input_path, 'rb') as f:
            image = Image.open(f)
            image.load()
        
        # 2. 执行分析（示例：获取图片信息）
        analysis_result = {
            "width": image.width,
            "height": image.height,
            "format": image.format,
            "mode": image.mode,
            "size_bytes": len(image.tobytes())
        }
        
        # 3. 将结果写入输出文件路径
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(analysis_result, f, indent=2, ensure_ascii=False)
        
        print(f"Analysis completed. Output written to: {output_path}")
        
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: analyze_image.py <input_file> <output_file>", file=sys.stderr)
        sys.exit(1)
    
    input_file = sys.argv[1]   # {input} 替换后的文件路径
    output_file = sys.argv[2]  # {output} 替换后的文件路径
    
    analyze_image(input_file, output_file)
```

**说明**:

1. **输入处理**: `{input}`会被Agent替换为已下载的输入文件的完整路径（例如: `C:\Users\...\AppData\Local\Temp\job_xxx_input`）
   - 脚本应该通过文件路径读取输入文件
   - 示例: `with open(input_path, 'rb') as f: ...`

2. **输出处理**: `{output}`会被Agent替换为输出文件的完整路径（例如: `C:\Users\...\AppData\Local\Temp\job_xxx_output`）
   - 脚本应该将结果写入此文件路径
   - 示例: `with open(output_path, 'w') as f: json.dump(result, f)`
   - **重要**: Agent会在执行完命令后读取此文件并上传到OSS

3. **命令执行流程**:
   ```
   Agent下载输入 → 替换占位符 → 执行命令 → 读取输出文件 → 上传到OSS
   ```
   
4. **备用方案**: 如果输出文件不存在，Agent会尝试使用命令的stdout作为输出（但建议始终写入输出文件）

5. **临时文件**: Agent会自动清理临时文件，脚本无需手动清理

### 示例2: 查询作业状态

```bash
curl http://localhost:8080/api/jobs/550e8400-e29b-41d4-a716-446655440000
```

响应:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "SUCCEEDED",
  "output_key": "jobs/550e8400-e29b-41d4-a716-446655440000/1/output.json",
  ...
}
```

### 示例3: 轮询作业状态直到完成

```python
import requests
import time

job_id = "550e8400-e29b-41d4-a716-446655440000"
base_url = "http://localhost:8080"

while True:
    response = requests.get(f"{base_url}/api/jobs/{job_id}")
    job = response.json()
    
    if job["status"] in ["SUCCEEDED", "FAILED", "CANCELED", "LOST"]:
        print(f"Job completed with status: {job['status']}")
        if job["status"] == "SUCCEEDED":
            if job.get('output_key'):
                print(f"Output key: {job['output_key']}")
            else:
                print("No output file (stdout only)")
            if job.get('stdout'):
                print(f"Stdout: {job['stdout']}")
        elif job["status"] == "FAILED":
            if job.get('stderr'):
                print(f"Stderr: {job['stderr']}")
        break
    
    time.sleep(2)  # 等待2秒后重试
```

---

## 架构原则

### 控制平面 vs 数据平面

- **控制平面**: 所有HTTP API消息必须小（JSON格式，最大1MB）
- **数据平面**: 所有大文件必须通过OSS传输，使用presigned URL或STS临时凭证
- **禁止**: Cloud服务器不接受文件上传作为请求体

### 作业生命周期

1. **创建**: 通过 `POST /api/jobs` 创建，状态为 `PENDING`
2. **分配**: 调度器将作业分配给在线Agent，状态变为 `ASSIGNED`
3. **执行**: Agent开始执行，状态变为 `RUNNING`
4. **完成**: 状态变为 `SUCCEEDED` 或 `FAILED`
5. **输出**: 成功时，输出文件位于OSS的 `output_key` 或 `output_prefix` 下

### 输出路径规则

- 默认格式: `jobs/{job_id}/{attempt_id}/output.{extension}`
- 扩展名由 `output_extension` 指定（默认: `bin`）
- 如果指定了 `output_key`，必须符合上述格式
- 如果指定了 `output_prefix`，必须符合上述格式
- Agent会将输出文件写入此路径
- **注意**: 如果命令不产生输出文件（仅stdout），`output_key` 可能为空

---

## 错误处理

### 常见错误

1. **缺少必需字段**
   ```json
   {
     "error": "input_bucket and input_key are required (OSS keys only, not file content)"
   }
   ```

2. **命令过长**
   ```json
   {
     "error": "command exceeds maximum length of 8192 characters"
   }
   ```

3. **无效的Content-Type**
   ```json
   {
     "error": "Content-Type must be application/json. Only OSS keys are accepted, not file content."
   }
   ```

4. **请求体过大**
   ```json
   {
     "error": "Request body exceeds maximum size of 1048576 bytes. Only OSS keys are accepted, not file content."
   }
   ```

---

## 注意事项

1. **命令执行**: `command` 字段是可选的，但如果未提供，Agent将返回错误。建议始终提供命令。

2. **占位符**: 命令中的 `{input}` 和 `{output}` 会被Agent自动替换为实际文件路径。

3. **并发控制**: Agent的 `max_concurrency` 限制了同时执行的作业数。如果所有Agent都达到上限，新作业将保持 `PENDING` 状态。

4. **作业重试**: 当前MVP版本不支持自动重试。如果需要重试，需要创建新的作业（使用不同的 `attempt_id`）。

5. **输出文件**: 
   - Agent执行命令后，如果命令产生输出文件，必须将输出写入 `{output}` 指定的文件路径
   - 如果输出文件不存在，Agent将使用stdout作为输出数据
   - 如果命令不产生输出文件（仅stdout），`output_key` 可以为空，结果通过 `stdout` 字段返回
6. **stdout/stderr**: 
   - 所有命令执行的stdout和stderr都会被保存（截断到10KB）
   - 在查询作业状态时，可以通过 `stdout` 和 `stderr` 字段查看
   - 失败时，`stderr` 通常包含错误信息

6. **文件路径格式**: 
   - `{input}` 和 `{output}` 都是完整的文件系统路径
   - Windows示例: `C:\Users\...\AppData\Local\Temp\job_xxx_input`
   - Linux示例: `/tmp/job_xxx_input`
   - 脚本应该直接使用这些路径进行文件操作
