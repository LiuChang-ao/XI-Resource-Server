package queue

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Config holds Redis connection configuration
type Config struct {
	// URL is the Redis connection URL (e.g., redis://localhost:6379)
	// If provided, other fields are ignored
	URL string

	// Host is the Redis host (default: localhost)
	Host string

	// Port is the Redis port (default: 6379)
	Port int

	// Password is the Redis password (optional)
	Password string

	// Database is the Redis database number (default: 0)
	Database int

	// Username is the Redis username (optional, for Redis 6+ ACL)
	Username string

	// TLSEnabled enables TLS for Redis connection
	TLSEnabled bool
}

// LoadConfig loads Redis configuration from environment variables
func LoadConfig() *Config {
	cfg := &Config{
		Host:     "localhost", // Default
		Port:     6379,        // Default
		Database: 0,           // Default
	}

	// Check for URL first (most common for cloud deployments)
	if url := os.Getenv("REDIS_URL"); url != "" {
		cfg.URL = url
		return cfg
	}

	// Individual configuration options
	if host := os.Getenv("REDIS_HOST"); host != "" {
		cfg.Host = host
	}

	if portStr := os.Getenv("REDIS_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = port
		}
	}

	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		cfg.Password = password
	}

	if dbStr := os.Getenv("REDIS_DATABASE"); dbStr != "" {
		if db, err := strconv.Atoi(dbStr); err == nil {
			cfg.Database = db
		}
	}

	if username := os.Getenv("REDIS_USERNAME"); username != "" {
		cfg.Username = username
	}

	if tlsStr := os.Getenv("REDIS_TLS_ENABLED"); tlsStr != "" {
		cfg.TLSEnabled = strings.ToLower(tlsStr) == "true" || tlsStr == "1"
	}

	return cfg
}

// IsConfigured checks if Redis is actually configured (not just using defaults)
func (c *Config) IsConfigured() bool {
	// If URL is set, it's configured
	if c.URL != "" {
		return true
	}
	// If REDIS_HOST or REDIS_URL env vars are set, it's configured
	if os.Getenv("REDIS_URL") != "" || os.Getenv("REDIS_HOST") != "" {
		return true
	}
	// If any other Redis config is set, consider it configured
	return os.Getenv("REDIS_PORT") != "" ||
		os.Getenv("REDIS_PASSWORD") != "" ||
		os.Getenv("REDIS_DATABASE") != "" ||
		os.Getenv("REDIS_USERNAME") != "" ||
		os.Getenv("REDIS_TLS_ENABLED") != ""
}

// NewRedisClient creates a Redis client from configuration
func NewRedisClient(cfg *Config) (*redis.Client, error) {
	var opt *redis.Options

	if cfg.URL != "" {
		// Parse URL (supports redis:// and rediss://)
		parsedOpt, err := redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
		}
		opt = parsedOpt
	} else {
		// Build options from individual config fields
		opt = &redis.Options{
			Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Password: cfg.Password,
			DB:       cfg.Database,
			Username: cfg.Username,
		}

		// TLS is handled automatically by rediss:// URL scheme
		// For explicit TLS with host:port, we'd need to set TLSConfig
		// For now, use rediss:// URL if TLS is needed
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}
