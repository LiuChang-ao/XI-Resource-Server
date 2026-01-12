package job

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// DBConfig holds database configuration
type DBConfig struct {
	// Type is the database type: "mysql" or "sqlite" (default: "sqlite")
	Type string

	// MySQL configuration (used when Type == "mysql")
	MySQLHost     string
	MySQLPort     int
	MySQLUser     string
	MySQLPassword string
	MySQLDatabase string
	MySQLParams   string // Additional connection parameters (e.g., "charset=utf8mb4&parseTime=True&loc=Local")

	// SQLite configuration (used when Type == "sqlite")
	SQLitePath string
}

// LoadConfigFromEnv loads database configuration from environment variables or .env file
// It first tries to load from .env file (if exists), then reads from environment variables
// Environment variables (can be set in .env file or system environment):
//   - DB_TYPE: Database type ("mysql" or "sqlite", default: "sqlite")
//   - MYSQL_HOST: MySQL host (required if DB_TYPE=mysql)
//   - MYSQL_PORT: MySQL port (default: 3306)
//   - MYSQL_USER: MySQL username (required if DB_TYPE=mysql)
//   - MYSQL_PASSWORD: MySQL password (required if DB_TYPE=mysql)
//   - MYSQL_DATABASE: MySQL database name (required if DB_TYPE=mysql)
//   - MYSQL_PARAMS: Additional MySQL connection parameters (optional)
//   - SQLITE_PATH: SQLite database file path (default: "jobs.db", used if DB_TYPE=sqlite or not configured)
func LoadConfigFromEnv() (*DBConfig, error) {
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

	cfg := &DBConfig{}

	// Get database type
	dbType := strings.ToLower(strings.TrimSpace(os.Getenv("DB_TYPE")))
	if dbType == "" {
		dbType = "sqlite" // Default to SQLite
	}
	cfg.Type = dbType

	if dbType == "mysql" {
		// Load MySQL configuration
		cfg.MySQLHost = os.Getenv("MYSQL_HOST")
		cfg.MySQLUser = os.Getenv("MYSQL_USER")
		cfg.MySQLPassword = os.Getenv("MYSQL_PASSWORD")
		cfg.MySQLDatabase = os.Getenv("MYSQL_DATABASE")
		cfg.MySQLParams = os.Getenv("MYSQL_PARAMS")

		// Validate required MySQL fields
		if cfg.MySQLHost == "" {
			return nil, fmt.Errorf("MYSQL_HOST environment variable is required when DB_TYPE=mysql")
		}
		if cfg.MySQLUser == "" {
			return nil, fmt.Errorf("MYSQL_USER environment variable is required when DB_TYPE=mysql")
		}
		if cfg.MySQLPassword == "" {
			return nil, fmt.Errorf("MYSQL_PASSWORD environment variable is required when DB_TYPE=mysql")
		}
		if cfg.MySQLDatabase == "" {
			return nil, fmt.Errorf("MYSQL_DATABASE environment variable is required when DB_TYPE=mysql")
		}

		// Parse port
		portStr := os.Getenv("MYSQL_PORT")
		if portStr == "" {
			cfg.MySQLPort = 3306 // Default MySQL port
		} else {
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, fmt.Errorf("invalid MYSQL_PORT: %w", err)
			}
			cfg.MySQLPort = port
		}

		// Set default MySQL params if not provided
		if cfg.MySQLParams == "" {
			cfg.MySQLParams = "charset=utf8mb4&parseTime=True&loc=Local"
		}
	} else {
		// SQLite configuration
		sqlitePath := os.Getenv("SQLITE_PATH")
		if sqlitePath == "" {
			sqlitePath = "jobs.db" // Default SQLite path
		}
		cfg.SQLitePath = sqlitePath
	}

	return cfg, nil
}

// IsConfigured checks if MySQL is actually configured (not just using defaults)
func (c *DBConfig) IsMySQLConfigured() bool {
	return c.Type == "mysql" && c.MySQLHost != "" && c.MySQLUser != "" && c.MySQLPassword != "" && c.MySQLDatabase != ""
}
