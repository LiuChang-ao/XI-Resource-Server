# OSS Provider Module

This module provides a unified interface for generating presigned URLs for object storage access, specifically designed for Tencent Cloud COS (Object Storage Service).

## Features

- **Presigned URL Generation**: Generate short-lived presigned URLs for both download (GET) and upload (PUT) operations
- **Minimal Permissions**: URLs are scoped to specific keys or prefixes, ensuring least-privilege access
- **Configurable Expiration**: Default 15-minute expiration, configurable via environment variables or config struct
- **No Agent Credentials**: Agents never need to hold permanent AK/SK credentials

## Usage

### Basic Usage

```go
import (
    "context"
    "time"
    "github.com/xiresource/cloud/internal/oss"
)

// Create provider with explicit config
config := oss.Config{
    SecretID:   "your-secret-id",
    SecretKey:  "your-secret-key",
    Bucket:     "your-bucket",
    Region:     "ap-beijing",
    PresignTTL: 15 * time.Minute,
}

provider, err := oss.NewCOSProvider(config)
if err != nil {
    log.Fatal(err)
}

ctx := context.Background()

// Generate download URL for input
downloadURL, err := provider.GenerateDownloadURL(ctx, "inputs/job-123/data.zip")
if err != nil {
    log.Fatal(err)
}

// Generate upload URL for output
uploadURL, err := provider.GenerateUploadURL(ctx, "jobs/job-123/attempt-1/output.zip")
if err != nil {
    log.Fatal(err)
}
```

### Using Environment Variables or .env File

The `LoadConfigFromEnv()` function automatically looks for a `.env` file in the current directory or parent directories (up to 3 levels), then reads from system environment variables.

#### Using .env File (Recommended for local development)

1. Create a `.env` file in the `cloud/` directory or project root:

```bash
# .env file
COS_SECRET_ID=your-secret-id
COS_SECRET_KEY=your-secret-key
COS_BUCKET=your-bucket
COS_REGION=ap-shanghai
COS_PRESIGN_TTL_MINUTES=15
```

2. Use it in your code:

```go
// Load configuration from .env file or environment variables
config, err := oss.LoadConfigFromEnv()
if err != nil {
    log.Fatal(err)
}

provider, err := oss.NewCOSProvider(config)
if err != nil {
    log.Fatal(err)
}
```

#### Using System Environment Variables

Set environment variables directly in your system or shell:

```powershell
# PowerShell
$env:COS_SECRET_ID = "your-secret-id"
$env:COS_SECRET_KEY = "your-secret-key"
$env:COS_BUCKET = "your-bucket"
$env:COS_REGION = "ap-shanghai"
```

```bash
# Bash
export COS_SECRET_ID=your-secret-id
export COS_SECRET_KEY=your-secret-key
export COS_BUCKET=your-bucket
export COS_REGION=ap-shanghai
```

**Configuration Variables:**

Required:
- `COS_SECRET_ID`: Tencent Cloud SecretID
- `COS_SECRET_KEY`: Tencent Cloud SecretKey
- `COS_BUCKET`: COS bucket name
- `COS_REGION`: COS region (e.g., "ap-beijing", "ap-shanghai")

Optional:
- `COS_PRESIGN_TTL_MINUTES`: Presigned URL expiration in minutes (default: 15)
- `COS_BASE_URL`: Custom base URL (auto-generated if not set)

**Note:** The `.env` file is automatically ignored by `.gitignore` to prevent committing sensitive credentials.

### Generating URLs with Prefix

For cases where the exact output key is not known in advance:

```go
// Generate upload URL with prefix
prefix := "jobs/job-123/attempt-1"
filename := "output.zip"
uploadURL, err := provider.GenerateUploadURLWithPrefix(ctx, prefix, filename)
```

This generates a URL for `jobs/job-123/attempt-1/output.zip`.

## Security Considerations

1. **Short Expiration**: Presigned URLs expire after a configurable duration (default 15 minutes)
2. **Scoped Access**: Each URL is scoped to a specific key or prefix
3. **No Permanent Credentials**: Agents never receive permanent AK/SK credentials
4. **HTTPS Only**: All presigned URLs use HTTPS

## Testing

Unit tests are provided in `oss_test.go`. To run tests with real COS credentials:

```powershell
$env:COS_SECRET_ID = "your-secret-id"
$env:COS_SECRET_KEY = "your-secret-key"
$env:COS_BUCKET = "your-bucket"
$env:COS_REGION = "ap-beijing"
go test ./internal/oss -v
```

Tests will be skipped if credentials are not set.

## Integration with Job Assignment

This module is designed to be used when assigning jobs to agents. Example:

```go
// When assigning a job to an agent
jobID := "job-123"
attemptID := 1
inputKey := fmt.Sprintf("inputs/%s/data.zip", jobID)
outputPrefix := fmt.Sprintf("jobs/%s/%d", jobID, attemptID)

// Generate presigned URLs
inputURL, _ := provider.GenerateDownloadURL(ctx, inputKey)
outputURL, _ := provider.GenerateUploadURLWithPrefix(ctx, outputPrefix, "result.zip")

// Include in JobAssigned message
jobAssigned := &control.JobAssigned{
    JobId: jobID,
    AttemptId: int32(attemptID),
    InputDownload: &control.OSSAccess{
        PresignedUrl: inputURL,
    },
    OutputUpload: &control.OSSAccess{
        PresignedUrl: outputURL,
    },
    OutputPrefix: outputPrefix,
}
```
