package oss

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCOSProvider_GenerateDownloadURL(t *testing.T) {
	// Skip if credentials are not set
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	bucket := os.Getenv("COS_BUCKET")
	region := os.Getenv("COS_REGION")

	if secretID == "" || secretKey == "" || bucket == "" || region == "" {
		t.Skip("Skipping test: COS credentials not set (set COS_SECRET_ID, COS_SECRET_KEY, COS_BUCKET, COS_REGION)")
	}

	config := Config{
		SecretID:   secretID,
		SecretKey:  secretKey,
		Bucket:     bucket,
		Region:     region,
		PresignTTL: 15 * time.Minute,
	}

	provider, err := NewCOSProvider(config)
	if err != nil {
		t.Fatalf("Failed to create COS provider: %v", err)
	}

	ctx := context.Background()
	testKey := "test/input.txt"

	url, err := provider.GenerateDownloadURL(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to generate download URL: %v", err)
	}

	if url == "" {
		t.Error("Generated URL is empty")
	}

	t.Logf("Generated download URL: %s", url)
}

func TestCOSProvider_GenerateUploadURL(t *testing.T) {
	// Skip if credentials are not set
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	bucket := os.Getenv("COS_BUCKET")
	region := os.Getenv("COS_REGION")

	if secretID == "" || secretKey == "" || bucket == "" || region == "" {
		t.Skip("Skipping test: COS credentials not set (set COS_SECRET_ID, COS_SECRET_KEY, COS_BUCKET, COS_REGION)")
	}

	config := Config{
		SecretID:   secretID,
		SecretKey:  secretKey,
		Bucket:     bucket,
		Region:     region,
		PresignTTL: 15 * time.Minute,
	}

	provider, err := NewCOSProvider(config)
	if err != nil {
		t.Fatalf("Failed to create COS provider: %v", err)
	}

	ctx := context.Background()
	testKey := "test/output.txt"

	url, err := provider.GenerateUploadURL(ctx, testKey)
	if err != nil {
		t.Fatalf("Failed to generate upload URL: %v", err)
	}

	if url == "" {
		t.Error("Generated URL is empty")
	}

	t.Logf("Generated upload URL: %s", url)
}

func TestCOSProvider_GenerateUploadURLWithPrefix(t *testing.T) {
	// Skip if credentials are not set
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	bucket := os.Getenv("COS_BUCKET")
	region := os.Getenv("COS_REGION")

	if secretID == "" || secretKey == "" || bucket == "" || region == "" {
		t.Skip("Skipping test: COS credentials not set (set COS_SECRET_ID, COS_SECRET_KEY, COS_BUCKET, COS_REGION)")
	}

	config := Config{
		SecretID:   secretID,
		SecretKey:  secretKey,
		Bucket:     bucket,
		Region:     region,
		PresignTTL: 15 * time.Minute,
	}

	provider, err := NewCOSProvider(config)
	if err != nil {
		t.Fatalf("Failed to create COS provider: %v", err)
	}

	ctx := context.Background()
	prefix := "jobs/job-123/attempt-1"
	filename := "output.zip"

	url, err := provider.GenerateUploadURLWithPrefix(ctx, prefix, filename)
	if err != nil {
		t.Fatalf("Failed to generate upload URL with prefix: %v", err)
	}

	if url == "" {
		t.Error("Generated URL is empty")
	}

	t.Logf("Generated upload URL: %s", url)
}

func TestCOSProvider_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "missing SecretID",
			config: Config{
				SecretKey: "key",
				Bucket:    "bucket",
				Region:    "region",
			},
			wantErr: true,
		},
		{
			name: "missing SecretKey",
			config: Config{
				SecretID: "id",
				Bucket:   "bucket",
				Region:   "region",
			},
			wantErr: true,
		},
		{
			name: "missing Bucket",
			config: Config{
				SecretID:  "id",
				SecretKey: "key",
				Region:    "region",
			},
			wantErr: true,
		},
		{
			name: "missing Region",
			config: Config{
				SecretID:  "id",
				SecretKey: "key",
				Bucket:    "bucket",
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: Config{
				SecretID:  "id",
				SecretKey: "key",
				Bucket:    "bucket",
				Region:    "ap-beijing",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCOSProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCOSProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCOSProvider_DefaultTTL(t *testing.T) {
	config := Config{
		SecretID:  "id",
		SecretKey: "key",
		Bucket:    "bucket",
		Region:    "ap-beijing",
		// PresignTTL not set, should default to 15 minutes
	}

	provider, err := NewCOSProvider(config)
	if err != nil {
		t.Fatalf("Failed to create COS provider: %v", err)
	}

	cosProvider := provider.(*COSProvider)
	if cosProvider.config.PresignTTL != 15*time.Minute {
		t.Errorf("Expected default TTL of 15 minutes, got %v", cosProvider.config.PresignTTL)
	}
}
