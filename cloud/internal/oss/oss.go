package oss

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// Config holds OSS provider configuration
type Config struct {
	SecretID   string        // Tencent Cloud SecretID
	SecretKey  string        // Tencent Cloud SecretKey
	Bucket     string        // COS bucket name
	Region     string        // COS region (e.g., "ap-beijing")
	PresignTTL time.Duration // Presigned URL expiration (default: 15 minutes)
	BaseURL    string        // Optional: custom base URL (if empty, auto-generated from bucket+region)
}

// Provider defines the interface for OSS access operations
type Provider interface {
	// GenerateDownloadURL generates a presigned GET URL for downloading an object
	// key: the object key in the bucket (e.g., "inputs/job-123/data.zip")
	// Returns: presigned URL string and error
	GenerateDownloadURL(ctx context.Context, key string) (string, error)

	// GenerateUploadURL generates a presigned PUT URL for uploading an object
	// key: the object key in the bucket (e.g., "jobs/job-123/attempt-1/output.zip")
	// Returns: presigned URL string and error
	GenerateUploadURL(ctx context.Context, key string) (string, error)

	// GenerateUploadURLWithPrefix generates a presigned PUT URL for uploading to a prefix
	// This is useful when the exact key is not known in advance
	// prefix: the key prefix (e.g., "jobs/job-123/attempt-1/")
	// filename: the filename to append to the prefix
	// Returns: presigned URL string and error
	GenerateUploadURLWithPrefix(ctx context.Context, prefix, filename string) (string, error)
}

// TestProvider extends Provider with methods needed for e2e testing
// These methods are only used in test code, not in production agent/cloud code
type TestProvider interface {
	Provider
	// ObjectExists checks if an object exists in the bucket (HEAD request)
	// Returns: true if object exists, false if not found, error for other failures
	ObjectExists(ctx context.Context, key string) (bool, error)
	// PutObject uploads an object directly (for test setup)
	PutObject(ctx context.Context, key string, data []byte) error
	// DeleteObject deletes an object (for test cleanup)
	DeleteObject(ctx context.Context, key string) error
}

// COSProvider implements Provider using Tencent Cloud COS
type COSProvider struct {
	client *cos.Client
	config Config
}

// NewCOSProvider creates a new COS provider instance
func NewCOSProvider(config Config) (Provider, error) {
	if config.SecretID == "" || config.SecretKey == "" {
		return nil, fmt.Errorf("SecretID and SecretKey are required")
	}
	if config.Bucket == "" || config.Region == "" {
		return nil, fmt.Errorf("Bucket and Region are required")
	}

	// Set default TTL if not specified
	if config.PresignTTL == 0 {
		config.PresignTTL = 15 * time.Minute
	}

	// Build base URL
	var baseURL *url.URL
	var err error
	if config.BaseURL != "" {
		baseURL, err = url.Parse(config.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid BaseURL: %w", err)
		}
	} else {
		// Auto-generate from bucket and region
		baseURL, err = url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", config.Bucket, config.Region))
		if err != nil {
			return nil, fmt.Errorf("failed to build base URL: %w", err)
		}
	}

	// Create COS client
	b := &cos.BaseURL{BucketURL: baseURL}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  config.SecretID,
			SecretKey: config.SecretKey,
		},
	})

	return &COSProvider{
		client: client,
		config: config,
	}, nil
}

// GenerateDownloadURL generates a presigned GET URL for downloading an object
func (p *COSProvider) GenerateDownloadURL(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}

	presignedURL, err := p.client.Object.GetPresignedURL(
		ctx,
		http.MethodGet,
		key,
		p.config.SecretID,
		p.config.SecretKey,
		p.config.PresignTTL,
		nil, // Optional parameters
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate download URL: %w", err)
	}

	return presignedURL.String(), nil
}

// GenerateUploadURL generates a presigned PUT URL for uploading an object
func (p *COSProvider) GenerateUploadURL(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key cannot be empty")
	}

	presignedURL, err := p.client.Object.GetPresignedURL(
		ctx,
		http.MethodPut,
		key,
		p.config.SecretID,
		p.config.SecretKey,
		p.config.PresignTTL,
		nil, // Optional parameters
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate upload URL: %w", err)
	}

	return presignedURL.String(), nil
}

// GenerateUploadURLWithPrefix generates a presigned PUT URL for uploading to a prefix
func (p *COSProvider) GenerateUploadURLWithPrefix(ctx context.Context, prefix, filename string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("prefix cannot be empty")
	}
	if filename == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	// Ensure prefix ends with /
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	key := prefix + filename
	return p.GenerateUploadURL(ctx, key)
}

// ObjectExists checks if an object exists in the bucket (HEAD request)
func (p *COSProvider) ObjectExists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, fmt.Errorf("key cannot be empty")
	}

	exists, err := p.client.Object.IsExist(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}
	return exists, nil
}

// PutObject uploads an object directly (for test setup)
func (p *COSProvider) PutObject(ctx context.Context, key string, data []byte) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	_, err := p.client.Object.Put(ctx, key, bytes.NewReader(data), nil)
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	return nil
}

// DeleteObject deletes an object (for test cleanup)
func (p *COSProvider) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	_, err := p.client.Object.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}
