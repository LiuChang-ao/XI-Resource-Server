# 图片分析作业完整操作流程

本文档详细描述了从用户上传图片到获取分析结果的完整操作流程。

## 概述

整个流程分为5个主要步骤：
1. **用户上传图片到OSS** - 用户（前端或手动HTTP调用）将图片上传到对象存储
2. **服务器创建作业** - 通过HTTP API创建图片分析作业（可指定输出文件扩展名）
3. **Agent获取并处理作业** - Agent从服务器获取作业，下载图片并执行分析
4. **Agent上报完成状态** - Agent完成分析后上报结果（包含stdout/stderr，output_key可选）
5. **用户轮询获取结果** - 用户通过HTTP API轮询作业状态，获取分析结果（可从OSS下载或直接查看stdout）

---

## 步骤1: 用户上传图片到OSS

### 说明
根据架构设计，**所有大文件必须通过OSS传输**。服务器不接受文件上传作为请求体。用户需要先将图片上传到OSS，然后通过OSS的bucket和key来创建作业。

**重要**: 本系统使用**腾讯云COS**（Cloud Object Storage），不是阿里云OSS。用户需要使用腾讯云COS SDK或工具上传文件。

### 操作方式

#### 方式A: 使用腾讯云COS SDK直接上传（推荐）

**Python示例** (使用腾讯云COS SDK):
```python
from qcloud_cos import CosConfig
from qcloud_cos import CosS3Client
import sys
import os

# 配置COS访问凭证（这些凭证应该从环境变量或密钥管理服务获取）
secret_id = os.getenv('COS_SECRET_ID')  # 腾讯云SecretID
secret_key = os.getenv('COS_SECRET_KEY')  # 腾讯云SecretKey
region = os.getenv('COS_REGION')  # 例如: 'ap-beijing'
bucket = os.getenv('COS_BUCKET')  # COS bucket名称

config = CosConfig(Region=region, SecretId=secret_id, SecretKey=secret_key)
client = CosS3Client(config)

# 上传图片
input_key = 'inputs/user123/image_20260112_103045.jpg'
with open('local_image.jpg', 'rb') as fp:
    response = client.put_object(
        Bucket=bucket,
        Body=fp,
        Key=input_key
    )

print(f"Image uploaded to: {input_key}")
print(f"ETag: {response['ETag']}")
```

**Go示例** (使用腾讯云COS Go SDK):
```go
package main

import (
    "context"
    "github.com/tencentyun/cos-go-sdk-v5"
    "net/http"
    "net/url"
    "os"
)

func uploadImage() error {
    // 从环境变量获取凭证
    secretID := os.Getenv("COS_SECRET_ID")
    secretKey := os.Getenv("COS_SECRET_KEY")
    bucket := os.Getenv("COS_BUCKET")
    region := os.Getenv("COS_REGION")  // 例如: "ap-beijing"
    
    // 构建COS客户端
    u, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region))
    b := &cos.BaseURL{BucketURL: u}
    client := cos.NewClient(b, &http.Client{
        Transport: &cos.AuthorizationTransport{
            SecretID:  secretID,
            SecretKey: secretKey,
        },
    })
    
    // 上传文件
    inputKey := "inputs/user123/image_20260112_103045.jpg"
    _, err := client.Object.PutFromFile(context.Background(), inputKey, "local_image.jpg", nil)
    return err
}
```

#### 方式B: 使用腾讯云COS控制台或命令行工具

```bash
# 使用腾讯云COS CLI (coscmd)
# 安装: pip install coscmd
coscmd config -r ap-beijing -b my-bucket
coscmd upload local_image.jpg inputs/user123/image_20260112_103045.jpg
```

#### 方式C: 使用PowerShell上传（Windows）

```powershell
# 使用腾讯云COS PowerShell SDK
# 需要先安装: Install-Module -Name QCloudCOS
$secretId = $env:COS_SECRET_ID
$secretKey = $env:COS_SECRET_KEY
$region = $env:COS_REGION
$bucket = $env:COS_BUCKET

# 上传文件
$inputKey = "inputs/user123/image_20260112_103045.jpg"
# 使用相应的PowerShell SDK方法上传
```

### 关键信息记录
上传完成后，需要记录以下信息用于创建作业：
- `input_bucket`: COS bucket名称（例如: `"my-bucket"`）
- `input_key`: COS对象key（例如: `"inputs/user123/image_20260112_103045.jpg"`）

**注意**: 
- 系统使用腾讯云COS，bucket和region需要与服务器配置的COS凭证匹配
- 服务器通过环境变量`COS_SECRET_ID`、`COS_SECRET_KEY`、`COS_BUCKET`、`COS_REGION`配置COS访问
- 用户上传时使用的bucket必须与服务器配置的bucket相同（或服务器有权限访问该bucket）

---

## 步骤2: 服务器创建图片分析作业

### HTTP API调用

**请求**:
```http
POST /api/jobs HTTP/1.1
Host: localhost:8080
Content-Type: application/json

{
  "input_bucket": "my-bucket",
  "input_key": "inputs/user123/image_20260112_103045.jpg",
  "output_bucket": "my-bucket",
  "output_extension": "json",
  "command": "python C:/scripts/analyze_image.py {input} {output}"
}
```

**cURL示例**:
```bash
curl -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input_bucket": "my-bucket",
    "input_key": "inputs/user123/image_20260112_103045.jpg",
    "output_bucket": "my-bucket",
    "output_extension": "json",
    "command": "python C:/scripts/analyze_image.py {input} {output}"
  }'
```

**Python示例**:
```python
import requests

response = requests.post(
    "http://localhost:8080/api/jobs",
    json={
        "input_bucket": "my-bucket",
        "input_key": "inputs/user123/image_20260112_103045.jpg",
        "output_bucket": "my-bucket",
        "output_extension": "json",  # 指定输出文件扩展名
        "command": "python C:/scripts/analyze_image.py {input} {output}"
    }
)

result = response.json()
job_id = result["job_id"]
print(f"Job created: {job_id}")
print(f"Status: {result['status']}")  # 应该是 "PENDING"
```

### 服务器处理流程

1. **接收请求**: API服务器接收POST请求，验证Content-Type为`application/json`
2. **验证参数**: 
   - 检查`input_bucket`和`input_key`是否存在
   - 检查`output_bucket`是否存在
   - 验证`command`长度不超过1024字符
   - **拒绝**`multipart/form-data`（防止文件上传绕过）
3. **创建作业记录**:
   - 生成`job_id` (UUID)
   - 设置`attempt_id = 1`
   - 设置状态为`PENDING`
   - 设置`output_prefix = "jobs/{job_id}/1/"`
   - 设置`output_extension`（如果未指定，默认为`"bin"`）
   - 保存到数据库（MySQL或SQLite）
4. **加入队列**: 将`job_id`加入Redis队列等待分配
5. **返回响应**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "PENDING",
  "created_at": "2026-01-12T10:30:45Z"
}
```

### 关键点
- **不传输文件内容**: 服务器只接收OSS key，不接收文件内容
- **作业状态**: 创建后状态为`PENDING`，等待Agent分配
- **输出路径**: 自动生成`jobs/{job_id}/{attempt_id}/`格式的输出前缀
- **输出扩展名**: 可通过`output_extension`指定（例如: `"json"`, `"txt"`），默认为`"bin"`

---

## 步骤3: Agent获取并处理作业

### 3.1 Agent连接和注册

Agent启动后会自动连接到服务器的WSS端点：

**Agent启动**:
```bash
agent.exe \
  --server ws://localhost:8080/wss \
  --agent-id agent-001 \
  --agent-token dev-token \
  --max-concurrency 1
```

**连接流程**:
1. Agent建立WebSocket连接（WSS over TLS，生产环境使用443端口）
2. Agent发送`Register`消息:
   ```protobuf
   Envelope {
     agent_id: "agent-001"
     request_id: "req-123"
     timestamp: 1705045845000
     payload: Register {
       agent_id: "agent-001"
       agent_token: "dev-token"
       hostname: "WORKSTATION-01"
       max_concurrency: 1
     }
   }
   ```
3. 服务器验证token（MVP开发模式接受任何token）
4. 服务器发送`RegisterAck`:
   ```protobuf
   RegisterAck {
     success: true
     message: "Registered"
     heartbeat_interval_sec: 20
   }
   ```
5. Agent收到`RegisterAck`后，开始发送心跳和请求作业

### 3.2 Agent请求作业

Agent在收到`RegisterAck`后，启动`jobRequestLoop`，定期发送`RequestJob`消息：

**RequestJob消息**:
```protobuf
Envelope {
  agent_id: "agent-001"
  request_id: "req-456"
  timestamp: 1705045850000
  payload: RequestJob {
    agent_id: "agent-001"
  }
}
```

**服务器处理RequestJob**:
1. 验证Agent状态:
   - Agent已注册且在线
   - Agent未暂停（`paused = false`）
   - Agent有容量（`running_jobs < max_concurrency`）
2. 从队列中取出作业:
   - 从Redis队列中`Dequeue`一个`job_id`
   - 验证作业状态为`PENDING`
   - 如果状态不是`PENDING`，跳过并尝试下一个
3. 生成Presigned URL:
   - **输入下载URL**: 使用COS Provider生成输入文件的presigned GET URL（有效期15分钟，可通过`COS_PRESIGN_TTL_MINUTES`配置）
   - **输出上传URL**: 生成输出文件的presigned PUT URL，目标key为`jobs/{job_id}/1/output.{extension}`（扩展名由作业的`output_extension`字段指定，默认`bin`）
4. 更新作业状态:
   - 设置`assigned_agent_id = "agent-001"`
   - 设置`lease_id`（UUID）
   - 更新状态为`ASSIGNED`
   - 更新`output_key`和`output_prefix`
5. 发送`JobAssigned`消息给Agent:
   ```protobuf
   Envelope {
     request_id: "req-456"
     timestamp: 1705045851000
     payload: JobAssigned {
       job_id: "550e8400-e29b-41d4-a716-446655440000"
       attempt_id: 1
       lease_id: "lease-uuid-123"
       lease_ttl_sec: 60
       input_download: {
         presigned_url: "https://my-bucket.cos.ap-beijing.myqcloud.com/inputs/user123/image.jpg?signature=..."
       }
       output_upload: {
         presigned_url: "https://my-bucket.cos.ap-beijing.myqcloud.com/jobs/550e8400.../1/output.json?signature=..."
       }
       output_prefix: "jobs/550e8400-e29b-41d4-a716-446655440000/1/"
       output_key: "jobs/550e8400-e29b-41d4-a716-446655440000/1/output.json"
       command: "python C:/scripts/analyze_image.py {input} {output}"
     }
   }
   ```

### 3.3 Agent处理作业

Agent收到`JobAssigned`后，执行以下步骤：

#### 3.3.1 检查容量
```go
if !c.canAcceptJob() {
    // 如果已暂停或达到最大并发数，拒绝作业
    c.reportJobStatus(jobID, attemptID, JOB_STATUS_FAILED, "Agent cannot accept job", "")
    return
}
c.incrementRunningJobs()  // 增加运行中作业计数
```

#### 3.3.2 上报RUNNING状态
```go
c.reportJobStatus(jobID, attemptID, JOB_STATUS_RUNNING, "Processing job", "")
```

服务器收到后更新作业状态为`RUNNING`。

#### 3.3.3 下载输入文件
```go
// 从presigned URL下载输入文件
// Agent会从input_key中提取扩展名并保留在临时文件名中
inputFile := "C:\\Users\\...\\AppData\\Local\\Temp\\job_550e8400..._input.jpg"
err := downloadInputToFile(inputDownloadURL, jobID, assigned.InputKey)
// 下载到临时文件: job_{job_id}_input{.<ext>}
// 例如: input_key = "inputs/image.jpg" → job_xxx_input.jpg
```

**下载过程**:
- 使用HTTP GET请求presigned URL
- 从`JobAssigned.input_key`中提取文件扩展名（如果存在）
- 将文件保存到临时目录，文件名格式: `job_{job_id}_input{.<ext>}`
  - Windows: `%TEMP%\job_{job_id}_input{.<ext>}`
  - Linux: `/tmp/job_{job_id}_input{.<ext>}`
- 验证HTTP状态码为200
- **重要**: 保留扩展名确保脚本可以正确识别文件类型（例如: Python脚本可以通过扩展名判断是图片还是文本文件）

#### 3.3.4 执行命令
```go
// 替换占位符
command := "python C:/scripts/analyze_image.py {input} {output}"
// 替换后:
command = "python C:/scripts/analyze_image.py C:\\Users\\...\\Temp\\job_xxx_input C:\\Users\\...\\Temp\\job_xxx_output"

// 执行命令，返回CommandResult（包含输出数据、stdout、stderr）
cmdResult, err := executeCommand(command, inputFile, outputFile)
```

**命令执行**:
- Windows: 使用`cmd.exe /C`执行命令
- 超时设置: 30分钟
- 占位符替换:
  - `{input}` → 输入文件完整路径
  - `{output}` → 输出文件完整路径
- **输出捕获**:
  - 自动捕获命令的stdout和stderr（截断到10KB）
  - 如果命令产生输出文件，读取文件内容
  - 如果输出文件不存在，使用stdout作为输出数据

**Python脚本示例** (`analyze_image.py`):
```python
#!/usr/bin/env python3
import sys
import json
from PIL import Image

def analyze_image(input_path, output_path):
    # 1. 读取输入图片
    with open(input_path, 'rb') as f:
        image = Image.open(f)
        image.load()
    
    # 2. 执行分析
    analysis_result = {
        "width": image.width,
        "height": image.height,
        "format": image.format,
        "mode": image.mode,
        "size_bytes": len(image.tobytes())
    }
    
    # 3. 写入输出文件
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(analysis_result, f, indent=2, ensure_ascii=False)
    
    print(f"Analysis completed. Output written to: {output_path}")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: analyze_image.py <input_file> <output_file>", file=sys.stderr)
        sys.exit(1)
    
    input_file = sys.argv[1]   # {input} 替换后的文件路径
    output_file = sys.argv[2]  # {output} 替换后的文件路径
    
    analyze_image(input_file, output_file)
```

**执行结果**:
- 脚本将分析结果写入`{output}`指定的文件
- Agent捕获命令的stdout和stderr（截断到10KB）
- Agent读取输出文件内容（如果存在）
- 如果输出文件不存在，Agent会尝试使用命令的stdout作为输出数据
- **重要**: 无论是否有输出文件，stdout和stderr都会被保存并在JobStatus中上报

#### 3.3.5 上传输出文件（如果存在）
```go
// 如果命令产生输出文件，上传到OSS
outputKeyToReport := ""
if cmdResult.HasOutputFile && len(cmdResult.OutputData) > 0 {
    // 使用presigned PUT URL上传输出
    err := uploadOutput(outputUploadURL, cmdResult.OutputData)
    if err != nil {
        // 上传失败，上报FAILED状态（携带stdout/stderr）
        c.reportJobStatusWithOutput(jobID, attemptID, JOB_STATUS_FAILED, 
            fmt.Sprintf("Upload failed: %v", err), "", cmdResult.Stdout, cmdResult.Stderr)
        return
    }
    outputKeyToReport = outputKey
} else {
    // 没有输出文件（仅stdout），output_key为空
    log.Printf("Job %s completed without output file (stdout only)", jobID)
}
```

**上传过程**:
- 仅当命令产生输出文件时才上传
- 使用HTTP PUT请求presigned URL
- Content-Type: `application/json`（对于JSON文件）或其他相应类型
- 验证HTTP状态码为200或204
- **注意**: 如果命令不产生输出文件，跳过上传步骤，`output_key`为空

#### 3.3.6 清理临时文件
```go
defer os.Remove(inputFile)   // 删除输入临时文件
defer os.Remove(outputFile)   // 删除输出临时文件
```

#### 3.3.7 上报SUCCEEDED状态
```go
c.reportJobStatus(jobID, attemptID, JOB_STATUS_SUCCEEDED, "", outputKey)
```

**JobStatus消息**:
```protobuf
Envelope {
  agent_id: "agent-001"
  request_id: "req-789"
  timestamp: 1705045900000
  payload: JobStatus {
    job_id: "550e8400-e29b-41d4-a716-446655440000"
    attempt_id: 1
    status: JOB_STATUS_SUCCEEDED
    message: ""
    output_key: "jobs/550e8400-e29b-41d4-a716-446655440000/1/output.bin"
  }
}
```

**服务器处理JobStatus**:
1. 验证作业属于该Agent
2. 验证`attempt_id`匹配
3. 更新作业状态为`SUCCEEDED`
4. 更新`output_key`字段
5. 更新Agent的`running_jobs`计数（减1）

---

## 步骤4: Agent上报完成状态

Agent在完成作业后自动上报状态（已在步骤3.3.7描述）。服务器更新作业状态后，用户可以通过API查询结果。

---

## 步骤5: 用户轮询获取结果

### 5.1 查询作业状态

**HTTP API调用**:
```http
GET /api/jobs/{job_id} HTTP/1.1
Host: localhost:8080
```

**cURL示例**:
```bash
curl http://localhost:8080/api/jobs/550e8400-e29b-41d4-a716-446655440000
```

**Python轮询示例**:
```python
import requests
import time

job_id = "550e8400-e29b-41d4-a716-446655440000"
base_url = "http://localhost:8080"

while True:
    response = requests.get(f"{base_url}/api/jobs/{job_id}")
    job = response.json()
    
    status = job["status"]
    print(f"Job status: {status}")
    
    # 检查是否完成（成功或失败）
    if status in ["SUCCEEDED", "FAILED", "CANCELED", "LOST"]:
        if status == "SUCCEEDED":
            print(f"Job completed successfully!")
            if job.get('output_key'):
                print(f"Output key: {job['output_key']}")
            else:
                print("No output file (stdout only)")
            print(f"Output prefix: {job['output_prefix']}")
            if job.get('stdout'):
                print(f"Stdout: {job['stdout']}")
        elif status == "FAILED":
            print(f"Job failed with status: {status}")
            if job.get('stderr'):
                print(f"Stderr: {job['stderr']}")
        else:
            print(f"Job completed with status: {status}")
        break
    
    # 等待2秒后重试
    time.sleep(2)
```

### 5.2 响应示例

**作业进行中**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2026-01-12T10:30:45Z",
  "status": "RUNNING",
  "input_bucket": "my-bucket",
  "input_key": "inputs/user123/image_20260112_103045.jpg",
  "output_bucket": "my-bucket",
  "output_key": "",
  "output_prefix": "jobs/550e8400-e29b-41d4-a716-446655440000/1/",
  "output_extension": "json",
  "attempt_id": 1,
  "assigned_agent_id": "agent-001",
  "lease_id": "lease-uuid-123",
  "command": "python C:/scripts/analyze_image.py {input} {output}",
  "stdout": "",
  "stderr": ""
}
```

**作业完成（有输出文件）**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2026-01-12T10:30:45Z",
  "status": "SUCCEEDED",
  "input_bucket": "my-bucket",
  "input_key": "inputs/user123/image_20260112_103045.jpg",
  "output_bucket": "my-bucket",
  "output_key": "jobs/550e8400-e29b-41d4-a716-446655440000/1/output.json",
  "output_prefix": "jobs/550e8400-e29b-41d4-a716-446655440000/1/",
  "output_extension": "json",
  "attempt_id": 1,
  "assigned_agent_id": "agent-001",
  "lease_id": "lease-uuid-123",
  "command": "python C:/scripts/analyze_image.py {input} {output}",
  "stdout": "Analysis completed. Output written to: C:\\...\\output.json",
  "stderr": ""
}
```

**作业完成（无输出文件，仅stdout）**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2026-01-12T10:30:45Z",
  "status": "SUCCEEDED",
  "input_bucket": "my-bucket",
  "input_key": "inputs/user123/image_20260112_103045.jpg",
  "output_bucket": "my-bucket",
  "output_key": "",
  "output_prefix": "jobs/550e8400-e29b-41d4-a716-446655440000/1/",
  "output_extension": "bin",
  "attempt_id": 1,
  "assigned_agent_id": "agent-001",
  "lease_id": "lease-uuid-123",
  "command": "python C:/scripts/check_status.py {input}",
  "stdout": "Image validation passed. Status: OK",
  "stderr": ""
}
```

**作业失败**:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2026-01-12T10:30:45Z",
  "status": "FAILED",
  "input_bucket": "my-bucket",
  "input_key": "inputs/user123/image_20260112_103045.jpg",
  "output_bucket": "my-bucket",
  "output_key": "",
  "output_prefix": "jobs/550e8400-e29b-41d4-a716-446655440000/1/",
  "output_extension": "json",
  "attempt_id": 1,
  "assigned_agent_id": "agent-001",
  "lease_id": "lease-uuid-123",
  "command": "python C:/scripts/analyze_image.py {input} {output}",
  "stdout": "Starting analysis...",
  "stderr": "Error: PIL.Image.UnidentifiedImageError: cannot identify image file"
}
```

### 5.3 获取结果

根据作业是否有输出文件，有两种方式获取结果：

#### 方式A: 从OSS下载结果文件（如果有output_key）

如果`output_key`不为空，用户可以从OSS下载分析结果：

**Python示例** (使用腾讯云COS SDK):
```python
from qcloud_cos import CosConfig
from qcloud_cos import CosS3Client
import json
import os

# 配置COS访问
secret_id = os.getenv('COS_SECRET_ID')
secret_key = os.getenv('COS_SECRET_KEY')
region = os.getenv('COS_REGION')
bucket = os.getenv('COS_BUCKET')

config = CosConfig(Region=region, SecretId=secret_id, SecretKey=secret_key)
client = CosS3Client(config)

# 下载结果
output_key = job["output_key"]
response = client.get_object(
    Bucket=bucket,
    Key=output_key
)

# 读取内容
result_data = response['Body'].read()

# 解析JSON结果
result = json.loads(result_data)
print(f"Image width: {result['width']}")
print(f"Image height: {result['height']}")
print(f"Image format: {result['format']}")
```

**Go示例** (使用腾讯云COS Go SDK):
```go
import (
    "context"
    "encoding/json"
    "github.com/tencentyun/cos-go-sdk-v5"
    "io/ioutil"
)

// 下载结果
outputKey := job["output_key"]
resp, err := client.Object.Get(context.Background(), outputKey, nil)
if err != nil {
    log.Fatal(err)
}
defer resp.Body.Close()

resultData, _ := ioutil.ReadAll(resp.Body)

// 解析JSON结果
var result map[string]interface{}
json.Unmarshal(resultData, &result)
fmt.Printf("Image width: %v\n", result["width"])
fmt.Printf("Image height: %v\n", result["height"])
fmt.Printf("Image format: %v\n", result["format"])
```

#### 方式B: 直接从API响应获取结果（如果output_key为空）

如果`output_key`为空（命令不产生输出文件），结果直接包含在API响应的`stdout`字段中：

```python
import requests

response = requests.get(f"{base_url}/api/jobs/{job_id}")
job = response.json()

if job["status"] == "SUCCEEDED":
    if job.get('output_key'):
        # 有输出文件，从OSS下载
        # ... (使用方式A的代码)
    else:
        # 无输出文件，直接使用stdout
        result_text = job.get('stdout', '')
        print(f"Command output: {result_text}")
        
        # 如果stdout是JSON格式，可以解析
        import json
        try:
            result = json.loads(result_text)
            print(f"Parsed result: {result}")
        except:
            # 不是JSON，直接使用文本
            pass
```

**注意**: 
- `stdout`和`stderr`字段最多包含10KB内容（超出部分会被截断）
- 如果命令输出很大，建议使用输出文件方式（指定`{output}`占位符）

---

## 完整流程图

```
┌─────────┐
│  用户   │
└────┬────┘
     │ 1. 上传图片到OSS
     ▼
┌─────────┐
│   OSS   │ (inputs/user123/image.jpg)
└────┬────┘
     │
     │ 2. POST /api/jobs (提供OSS key)
     ▼
┌─────────┐
│ 服务器  │ 创建作业 (PENDING) → 加入队列
└────┬────┘
     │
     │ 3. Agent请求作业 (RequestJob)
     ▼
┌─────────┐
│ Agent   │ 接收JobAssigned
└────┬────┘
     │
     │ 4. 下载输入 (presigned GET URL)
     ▼
┌─────────┐
│   OSS   │
└────┬────┘
     │
     │ 5. 执行分析命令
     ▼
┌─────────┐
│ Agent   │ 运行 analyze_image.py
└────┬────┘
     │
     │ 6. 上传输出 (presigned PUT URL，如果存在输出文件)
     ▼
┌─────────┐
│   OSS   │ (jobs/{job_id}/1/output.json) 或 无输出文件
└────┬────┘
     │
     │ 7. 上报SUCCEEDED状态（携带stdout/stderr）
     ▼
┌─────────┐
│ 服务器  │ 更新状态为SUCCEEDED，保存stdout/stderr
└────┬────┘
     │
     │ 8. GET /api/jobs/{job_id} (轮询)
     ▼
┌─────────┐
│  用户   │ 获取结果：
│         │ - 如果有output_key，从OSS下载输出
│         │ - 查看stdout获取命令输出
│         │ - 如果失败，查看stderr获取错误信息
└─────────┘
```

---

## 关键架构原则

### 控制平面 vs 数据平面分离

- **控制平面** (WSS/HTTP消息):
  - 所有消息必须小（JSON/Protobuf）
  - 最大请求体: 1MB
  - 只传输OSS key，不传输文件内容

- **数据平面** (COS):
  - 所有大文件通过腾讯云COS传输
  - 使用presigned URL（默认有效期15分钟，可通过`COS_PRESIGN_TTL_MINUTES`配置）
  - 服务器不代理文件内容
  - Agent直接与COS通信，不经过服务器

### 作业状态流转

```
PENDING → ASSIGNED → RUNNING → SUCCEEDED/FAILED
   │         │
   └─────────┘ (如果Agent拒绝或失败)
```

### 输出路径规则

- 默认格式: `jobs/{job_id}/{attempt_id}/`
- 输出文件: `jobs/{job_id}/{attempt_id}/output.{extension}`
  - 扩展名由`output_extension`指定（默认: `"bin"`）
  - 示例: `output.json`, `output.txt`, `output.bin`
- 确保幂等性: 使用`attempt_id`防止覆盖
- **无输出文件**: 如果命令不产生输出文件（仅stdout），`output_key`为空，结果通过`stdout`字段返回

---

## 错误处理

### 常见错误场景

1. **Agent拒绝作业**:
   - 原因: Agent暂停或达到最大并发数
   - 处理: Agent上报`FAILED`状态，作业保持`ASSIGNED`或回退到`PENDING`

2. **下载失败**:
   - 原因: presigned URL过期或网络问题
   - 处理: Agent上报`FAILED`状态

3. **命令执行失败**:
   - 原因: 脚本错误或超时
   - 处理: Agent上报`FAILED`状态，包含错误信息
   - **stderr**: 错误信息会保存在`stderr`字段中，可通过查询作业状态获取

4. **上传失败**:
   - 原因: presigned URL过期或网络问题
   - 处理: Agent上报`FAILED`状态

5. **Agent断开连接**:
   - 处理: 服务器检测到连接断开，如果租约过期，标记作业为`LOST`

---

## 测试建议

### 端到端测试流程

1. **准备环境**:
   ```bash
   # 启动服务器
   go run ./cloud/cmd/server --dev
   
   # 启动Agent
   go run ./agent/cmd/agent --server ws://localhost:8080/wss --agent-id agent-001
   ```

2. **上传测试图片到OSS**

3. **创建作业**:
   ```bash
   curl -X POST http://localhost:8080/api/jobs \
     -H "Content-Type: application/json" \
     -d '{"input_bucket":"my-bucket","input_key":"test/image.jpg","output_bucket":"my-bucket","output_extension":"json","command":"python C:/scripts/analyze.py {input} {output}"}'
   ```

4. **轮询状态**:
   ```bash
   curl http://localhost:8080/api/jobs/{job_id}
   ```

5. **验证结果**:
   - 检查作业状态为`SUCCEEDED`
   - 如果`output_key`不为空，从OSS下载输出文件验证内容
   - 检查`stdout`字段查看命令输出
   - 如果失败，检查`stderr`字段查看错误信息

---

## 总结

整个流程遵循"控制平面与数据平面分离"的架构原则：
- **控制平面**: 通过WSS/HTTP传输小消息（作业元数据、状态更新）
- **数据平面**: 通过腾讯云COS传输大文件（输入图片、输出结果）

这种设计确保了：
1. 服务器不承担文件传输负担
2. Agent可以直接从COS下载/上传，充分利用带宽
3. 系统可扩展，支持多个Agent并发处理作业
4. 使用presigned URL，Agent无需持有永久COS凭证，提高安全性

## 技术栈说明

- **对象存储**: 腾讯云COS (Cloud Object Storage)
- **COS配置**: 通过环境变量或`.env`文件配置
  - `COS_SECRET_ID`: 腾讯云SecretID
  - `COS_SECRET_KEY`: 腾讯云SecretKey
  - `COS_BUCKET`: COS bucket名称
  - `COS_REGION`: COS区域（如: `ap-beijing`, `ap-shanghai`）
  - `COS_PRESIGN_TTL_MINUTES`: Presigned URL有效期（分钟，默认15）

## 新功能说明

### 1. 输出文件格式自定义

创建作业时可以指定`output_extension`参数来控制输出文件的扩展名：

```json
{
  "input_bucket": "my-bucket",
  "input_key": "inputs/image.jpg",
  "output_bucket": "my-bucket",
  "output_extension": "json",  // 指定输出为JSON格式
  "command": "python C:/scripts/analyze.py {input} {output}"
}
```

输出文件将保存为：`jobs/{job_id}/1/output.json`

**支持的扩展名**: 任何有效的文件扩展名（不含点号），例如: `"json"`, `"txt"`, `"csv"`, `"bin"`等

### 2. 命令执行输出捕获

所有命令执行的stdout和stderr都会被自动捕获并保存：

- **stdout**: 命令的标准输出（截断到10KB）
- **stderr**: 命令的错误输出（截断到10KB）
- **保存时机**: 在作业状态更新时保存（RUNNING、SUCCEEDED、FAILED）
- **查询方式**: 通过`GET /api/jobs/{job_id}`接口的响应中的`stdout`和`stderr`字段获取

**使用场景**:
- 查看命令执行的详细输出
- 调试失败的命令（查看stderr）
- 获取不需要文件输出的命令结果（仅stdout）

### 3. 无输出文件的命令支持

如果命令不产生输出文件（仅输出到stdout），系统也支持：

```json
{
  "input_bucket": "my-bucket",
  "input_key": "inputs/data.txt",
  "output_bucket": "my-bucket",
  "command": "python C:/scripts/check_status.py {input}"  // 不指定{output}
}
```

**行为**:
- Agent执行命令后，如果输出文件不存在，`output_key`为空
- 命令的stdout会被保存并在查询时返回
- 作业状态仍为`SUCCEEDED`（如果命令成功执行）

**示例响应**:
```json
{
  "job_id": "...",
  "status": "SUCCEEDED",
  "output_key": "",  // 空，因为没有输出文件
  "stdout": "Validation passed. Status: OK",
  "stderr": ""
}
```
