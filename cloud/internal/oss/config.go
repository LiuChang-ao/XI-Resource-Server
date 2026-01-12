package oss

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// LoadConfigFromEnv loads OSS configuration from environment variables or .env file
// It first tries to load from .env file (if exists), then reads from environment variables
// Environment variables (can be set in .env file or system environment):
//   - COS_SECRET_ID: Tencent Cloud SecretID (required)
//   - COS_SECRET_KEY: Tencent Cloud SecretKey (required)
//   - COS_BUCKET: COS bucket name (required)
//   - COS_REGION: COS region (required, e.g., "ap-beijing")
//   - COS_PRESIGN_TTL_MINUTES: Presigned URL expiration in minutes (optional, default: 15)
//   - COS_BASE_URL: Custom base URL (optional, auto-generated if not set)
func LoadConfigFromEnv() (Config, error) {
	// Try to load .env file (ignore error if file doesn't exist)
	// First try current directory, then parent directories (up to 3 levels)
	// This allows .env file to be in cloud/ or project root
	for i := 0; i < 4; i++ {
		var envPath string
		if i == 0 {
			envPath = ".env"
		} else {
			envPath = filepath.Join(strings.Repeat("../", i), ".env")
		}

		// Try to load .env file (godotenv.Load returns error if file doesn't exist)
		if err := godotenv.Load(envPath); err == nil {
			break // Successfully loaded .env file
		}
	}
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	bucket := os.Getenv("COS_BUCKET")
	region := os.Getenv("COS_REGION")
	baseURL := os.Getenv("COS_BASE_URL")

	if secretID == "" {
		return Config{}, fmt.Errorf("COS_SECRET_ID environment variable is required")
	}
	if secretKey == "" {
		return Config{}, fmt.Errorf("COS_SECRET_KEY environment variable is required")
	}
	if bucket == "" {
		return Config{}, fmt.Errorf("COS_BUCKET environment variable is required")
	}
	if region == "" {
		return Config{}, fmt.Errorf("COS_REGION environment variable is required")
	}

	config := Config{
		SecretID:  secretID,
		SecretKey: secretKey,
		Bucket:    bucket,
		Region:    region,
		BaseURL:   baseURL,
	}

	// Parse TTL from environment
	if ttlStr := os.Getenv("COS_PRESIGN_TTL_MINUTES"); ttlStr != "" {
		ttlMinutes, err := strconv.Atoi(ttlStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid COS_PRESIGN_TTL_MINUTES: %w", err)
		}
		if ttlMinutes <= 0 {
			return Config{}, fmt.Errorf("COS_PRESIGN_TTL_MINUTES must be positive")
		}
		config.PresignTTL = time.Duration(ttlMinutes) * time.Minute
	} else {
		// Default to 15 minutes
		config.PresignTTL = 15 * time.Minute
	}

	return config, nil
}
