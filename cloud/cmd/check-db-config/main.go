package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/xiresource/cloud/internal/job"
)

func main() {
	// Try to load .env file
	for _, path := range []string{".env", "../.env", "../../.env"} {
		if err := godotenv.Load(path); err == nil {
			log.Printf("Loaded .env file from: %s", path)
			break
		}
	}

	// Load database configuration
	dbConfig, err := job.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load database config: %v", err)
	}

	fmt.Println("\n=== Database Configuration ===")
	fmt.Printf("Type: %s\n", dbConfig.Type)
	fmt.Printf("IsMySQLConfigured: %v\n", dbConfig.IsMySQLConfigured())

	if dbConfig.Type == "mysql" {
		fmt.Println("\n=== MySQL Configuration ===")
		fmt.Printf("Host: %s\n", dbConfig.MySQLHost)
		fmt.Printf("Port: %d\n", dbConfig.MySQLPort)
		fmt.Printf("User: %s\n", dbConfig.MySQLUser)
		fmt.Printf("Password: %s (hidden)\n", maskPassword(dbConfig.MySQLPassword))
		fmt.Printf("Database: %s\n", dbConfig.MySQLDatabase)
		fmt.Printf("Params: %s\n", dbConfig.MySQLParams)
	} else {
		fmt.Println("\n=== SQLite Configuration ===")
		fmt.Printf("Path: %s\n", dbConfig.SQLitePath)
	}

	fmt.Println("\n=== Environment Variables ===")
	fmt.Printf("DB_TYPE: %s\n", os.Getenv("DB_TYPE"))
	fmt.Printf("MYSQL_HOST: %s\n", os.Getenv("MYSQL_HOST"))
	fmt.Printf("MYSQL_PORT: %s\n", os.Getenv("MYSQL_PORT"))
	fmt.Printf("MYSQL_USER: %s\n", os.Getenv("MYSQL_USER"))
	fmt.Printf("MYSQL_PASSWORD: %s (hidden)\n", maskPassword(os.Getenv("MYSQL_PASSWORD")))
	fmt.Printf("MYSQL_DATABASE: %s\n", os.Getenv("MYSQL_DATABASE"))
	fmt.Printf("MYSQL_PARAMS: %s\n", os.Getenv("MYSQL_PARAMS"))
}

func maskPassword(pwd string) string {
	if pwd == "" {
		return "(empty)"
	}
	if len(pwd) <= 4 {
		return "****"
	}
	return pwd[:2] + "****" + pwd[len(pwd)-2:]
}
