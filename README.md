# Cloud-to-Lab Compute Bridge

A Go-based system that enables cloud services to securely dispatch compute jobs to lab Windows workstations via outbound WebSocket connections.

## Architecture

- **Cloud Server**: HTTP API + WSS Gateway + Agent Registry
- **Agent**: Windows service that maintains outbound WSS connection
- **Control Plane**: Protobuf messages over WebSocket
- **Data Plane**: OSS (Object Storage Service) for file transfers

## Prerequisites

- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)
- Go protobuf plugin: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`

## Setup

### Prerequisites
- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)
  - **Windows**: Download from [Protocol Buffers releases](https://github.com/protocolbuffers/protobuf/releases) (get `protoc-<version>-win64.zip`), extract and add `bin` folder to PATH
  - **macOS**: `brew install protobuf`
  - **Linux**: `sudo apt-get install protobuf-compiler` (Ubuntu/Debian) or use package manager
- Go protobuf plugin: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`

### Windows (PowerShell)

1. Generate protobuf code:
```powershell
# Option 1: Use the build script (recommended)
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 proto

# Option 2: Run directly (if execution policy allows)
& .\scripts\build.ps1 proto
```

Or manually:
```powershell
cd proto
protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto
cd ..
```

2. Setup Go workspace (for local module dependencies):
```powershell
# Create workspace in project root
go work init ./proto ./cloud ./agent

# Download dependencies
$env:GOPROXY = "https://goproxy.cn,direct"  # Use Chinese proxy if needed
cd cloud; go mod tidy; cd ..
cd agent; go mod tidy; cd ..
cd proto; go mod tidy; cd ..
```

3. Build cloud server:
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-cloud
```

4. Build agent:
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-agent
```

### Linux/macOS

1. Generate protobuf code:
```bash
make proto
```

2. Build cloud server:
```bash
make build-cloud
```

3. Build agent:
```bash
make build-agent
```

## Running Locally (Dev Mode)

### Start Cloud Server

```bash
cd cloud
go run cmd/server/main.go -addr :8080 -dev
```

The server will start with:
- HTTP API: `http://localhost:8080`
- WSS Gateway: `ws://localhost:8080/wss`
- Health check: `http://localhost:8080/health`
- Online agents: `http://localhost:8080/api/agents/online`

### Start Agent

In a separate terminal:

```bash
cd agent
go run cmd/agent/main.go -server ws://localhost:8080/wss -agent-id test-agent-001 -agent-token dev-token
```

## E2E Test

### Windows (PowerShell)
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 e2e
```

### Linux/macOS
```bash
make e2e
```

This will:
1. Start the cloud server
2. Start an agent
3. Wait for agent registration and heartbeat
4. Query the online agents API
5. Verify the agent appears in the list

## API Endpoints

### GET /api/agents/online

Returns a JSON array of online agents:

```json
[
  {
    "agent_id": "test-agent-001",
    "hostname": "workstation-01",
    "max_concurrency": 1,
    "paused": false,
    "running_jobs": 0,
    "last_heartbeat": "2024-01-01T12:00:00Z",
    "connected_at": "2024-01-01T11:59:00Z"
  }
]
```

### GET /health

Health check endpoint.

## Development Notes

- Dev mode (`-dev` flag) disables TLS and allows all WebSocket origins
- Agent sends Register message on connection
- Agent sends Heartbeat every 20 seconds (configurable)
- Agents are considered offline if no heartbeat received within 60 seconds

## Project Structure

```
.
├── cloud/          # Cloud server (HTTP API + WSS Gateway)
├── agent/          # Agent client (Windows service)
├── proto/          # Protobuf definitions
├── scripts/        # E2E test scripts
└── infra/          # Docker compose for local dev
```

## Next Steps (Future)

- Add TLS support for production
- Implement job scheduling
- Add OSS integration for file transfers
- Windows Service wrapper for agent
