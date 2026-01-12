# Theme: Cloud-to-Lab Compute Bridge (Go)

## Context
We run AI training/inference on lab Windows 11 workstations (RTX 4090, no public IP, no stable private IP, behind campus NAT/captive portal).
Cloud side is a small Aliyun ECS with public IPv4 (~5Mbps), plus managed MySQL, Redis, and OSS.

## Goal
Enable cloud services (Web/API) to securely and reliably dispatch compute jobs to lab workstations.
Data plane uses OSS: users upload to OSS; agents download inputs from OSS, run compute, upload outputs back to OSS.

## Non-goals (for MVP)
- No direct inbound access to lab machines from the internet.
- No remote desktop/SSH solution.
- No full multi-tenant RBAC UI (basic auth/token is enough).
- No automatic retry for long training jobs (avoid duplicated costs).

## Key Architectural Decisions (Must follow)
1) Outbound-only connectivity from lab machines:
   - Agents MUST initiate and maintain outbound TLS connection to cloud (WSS over 443).
   - Cloud MUST NOT require fixed private IPs or inbound ports to lab network.

2) Separation of Control Plane and Data Plane:
   - Control plane: WSS/HTTP messages only (small payloads).
   - Data plane: all large files MUST go through OSS using presigned URLs or STS.
   - ECS MUST NOT proxy file content.

3) Job lifecycle and idempotency:
   - Jobs have states: PENDING -> ASSIGNED -> RUNNING -> SUCCEEDED/FAILED/CANCELED
   - Lease mechanism for liveness; output paths include job_id + attempt_id to prevent overwrite.

4) Windows-first agent:
   - Agent runs as Windows Service and must survive reboots.
   - Provide pause/resume to avoid interfering with interactive desktop usage.

## MVP Acceptance Criteria (Must be testable)
A) Agent Online:
   - After starting agent.exe, server reports ONLINE within 30 seconds.

B) End-to-end job:
   - Create job via HTTP API; agent downloads input from OSS, produces output, uploads to OSS; server shows SUCCEEDED and output keys.

C) No cloud bandwidth for file payload:
   - ECS logs confirm that file bytes are not relayed.

## Repository Layout
- cloud/: Go server (HTTP API + WSS gateway + scheduler)
- agent/: Go agent (Windows service)
- proto/: Protobuf for control messages
- infra/: local dev (redis, mysql/sqlite)
- scripts/: e2e tools

## Coding Principles
- Keep MVP minimal and testable.
- Prefer explicit interfaces and small packages.
- Every change must add/adjust tests and keep `scripts/e2e.go` passing.

## When unsure
- Follow `.cursor/rules/*` documents first.
- If a feature risks violating "OSS-only data plane", stop and refactor.
