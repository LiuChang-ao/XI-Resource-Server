# E2E Test with Real OSS (Tencent Cloud COS)

This e2e test verifies the complete job flow using real OSS (Tencent Cloud COS):
1. Upload input object to OSS
2. Start cloud server and agent
3. Create job via API
4. Wait for job completion (agent downloads input, processes, uploads output)
5. Verify output object exists in OSS
6. Clean up test objects

## Prerequisites

- OSS credentials configured via environment variables
- Go 1.21 or later
- Protobuf compiler (`protoc`)

## Environment Variables

The test supports environment variables via:
1. **.env file** (recommended for local development) - The test automatically loads `.env` files from (in order):
   - Current directory (`scripts/.env`)
   - **`cloud/.env`** (most common location)
   - Project root (`.env`)
   - Parent directories
2. **System environment variables** - Set directly in your shell

### E2E-specific variables (recommended for testing):
- `E2E_OSS_ACCESS_KEY_ID` - Tencent Cloud SecretID
- `E2E_OSS_ACCESS_KEY_SECRET` - Tencent Cloud SecretKey
- `E2E_OSS_BUCKET` - COS bucket name
- `E2E_OSS_REGION` - COS region (e.g., "ap-shanghai")
- `E2E_OSS_PREFIX` - Optional, test object prefix (default: "e2e/test/")
- `E2E_OSS_ENDPOINT` - Optional, custom endpoint URL

### Fallback to COS_* variables:
If E2E_* variables are not set, the test will use:
- `COS_SECRET_ID` (instead of E2E_OSS_ACCESS_KEY_ID)
- `COS_SECRET_KEY` (instead of E2E_OSS_ACCESS_KEY_SECRET)
- `COS_BUCKET` (instead of E2E_OSS_BUCKET)
- `COS_REGION` (instead of E2E_OSS_REGION)
- `COS_BASE_URL` (instead of E2E_OSS_ENDPOINT)
- `COS_PRESIGN_TTL_MINUTES` - Optional, presigned URL TTL (default: 15)

### Example .env file:
Create a `.env` file in the `cloud/` directory (or project root):

```env
# E2E OSS Test Configuration
E2E_OSS_ACCESS_KEY_ID=your-secret-id
E2E_OSS_ACCESS_KEY_SECRET=your-secret-key
E2E_OSS_BUCKET=your-bucket
E2E_OSS_REGION=ap-shanghai
E2E_OSS_PREFIX=e2e/test/

# Or use COS_* variables (fallback)
# COS_SECRET_ID=your-secret-id
# COS_SECRET_KEY=your-secret-key
# COS_BUCKET=your-bucket
# COS_REGION=ap-shanghai
```

**Note:** The test will automatically find `.env` files in `cloud/`, `scripts/`, or the project root directory.

## Running the Test

### Prerequisites

**Important:** The test uses pre-built binaries from the `bin/` directory. Make sure to build them first:

**Windows (PowerShell):**
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-cloud
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-agent
```

**Linux/macOS:**
```bash
make build-cloud
make build-agent
```

Alternatively, the test will automatically build binaries if they don't exist (but this may have issues in some environments).

### Option 1: Using .env file (Recommended)

Create a `.env` file in the `cloud/` directory (or project root, or `scripts/` directory) with your OSS credentials:

```env
E2E_OSS_ACCESS_KEY_ID=your-secret-id
E2E_OSS_ACCESS_KEY_SECRET=your-secret-key
E2E_OSS_BUCKET=your-bucket
E2E_OSS_REGION=ap-shanghai
E2E_OSS_PREFIX=e2e/test/
```

Then simply run:

**Windows (PowerShell):**
```powershell
# Build binaries first (required)
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-cloud
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 build-agent

# Run test (will use pre-built binaries from bin/)
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 e2e-oss
```

Note: The `e2e-oss` target automatically builds binaries if needed.

**Linux/macOS:**
```bash
# Build binaries first (required)
make build-cloud
make build-agent

# Run test (will use pre-built binaries from bin/)
make e2e-oss
```

Note: The `e2e-oss` target automatically builds binaries if needed.

### Option 2: Using environment variables

**Windows (PowerShell):**
```powershell
# Set environment variables
$env:E2E_OSS_ACCESS_KEY_ID = "your-secret-id"
$env:E2E_OSS_ACCESS_KEY_SECRET = "your-secret-key"
$env:E2E_OSS_BUCKET = "your-bucket"
$env:E2E_OSS_REGION = "ap-shanghai"
$env:E2E_OSS_PREFIX = "e2e/test/"  # Optional

# Run the test
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1 e2e-oss
```

**Linux/macOS:**
```bash
# Set environment variables
export E2E_OSS_ACCESS_KEY_ID="your-secret-id"
export E2E_OSS_ACCESS_KEY_SECRET="your-secret-key"
export E2E_OSS_BUCKET="your-bucket"
export E2E_OSS_REGION="ap-shanghai"
export E2E_OSS_PREFIX="e2e/test/"  # Optional

# Run the test
make e2e-oss
```

### Direct execution:
```bash
cd scripts
go run e2e_oss.go
```

## Test Behavior

### Skip if credentials not configured:
If OSS credentials are not set, the test will:
- Print a warning message
- Exit with code 0 (success, indicating skip)
- Show instructions on how to configure credentials

### Test objects:
- Input objects: `{prefix}inputs/{timestamp}/input.bin`
- Output objects: `jobs/{job_id}/{attempt_id}/output.json` (or similar)

### Cleanup:
The test automatically cleans up:
- Input object (uploaded at test start)
- Output object (created by agent)

If cleanup fails, warnings are logged but the test still passes if all assertions succeeded.

## Verification

The test verifies:
1. ✅ Job is created successfully
2. ✅ Agent pulls and processes the job
3. ✅ Job status becomes `SUCCEEDED`
4. ✅ Output object exists in OSS at the expected key
5. ✅ Output content contains the job_id (optional verification)

## Troubleshooting

### Test fails with "OSS credentials not configured":
- Set the required environment variables (see above)
- Ensure variables are set in the same shell session where you run the test

### Test fails with "Output object does not exist":
- Check agent logs for upload errors
- Verify OSS permissions allow agent to upload to the output prefix
- Check that presigned URLs are valid (not expired)

### Test hangs waiting for job completion:
- Check agent is running and connected
- Verify agent can download from input presigned URL
- Check cloud server logs for job assignment errors

### Cleanup warnings:
- Cleanup failures don't fail the test
- Manually delete test objects if needed:
  - Input: `{prefix}inputs/{timestamp}/input.bin`
  - Output: `jobs/{job_id}/{attempt_id}/output.json`

## Notes

- The test uses port 8081 (different from M0_e2e.go which uses 8080)
- Test objects are prefixed with `e2e/test/` by default to avoid conflicts
- The test runs in dev mode (no TLS) for local testing
- All test objects are cleaned up automatically on success or failure
