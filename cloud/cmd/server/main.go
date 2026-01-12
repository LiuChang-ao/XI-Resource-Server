package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/xiresource/cloud/internal/api"
	"github.com/xiresource/cloud/internal/gateway"
	"github.com/xiresource/cloud/internal/job"
	"github.com/xiresource/cloud/internal/oss"
	"github.com/xiresource/cloud/internal/queue"
	"github.com/xiresource/cloud/internal/registry"
)

func main() {
	var (
		addr    = flag.String("addr", ":8080", "HTTP server address")
		wssPath = flag.String("wss-path", "/wss", "WebSocket path")
		devMode = flag.Bool("dev", false, "Development mode (no TLS)")
		dbPath  = flag.String("db", "jobs.db", "SQLite database path")
		envFile = flag.String("env", "", "Path to .env file (optional, will try to load automatically)")
	)
	flag.Parse()

	// Load environment variables from .env file if specified or try to auto-detect
	if *envFile != "" {
		if err := godotenv.Load(*envFile); err != nil {
			log.Printf("Warning: Failed to load .env file from %s: %v", *envFile, err)
		} else {
			log.Printf("Loaded .env file from %s", *envFile)
		}
	} else {
		// Try to load .env from current directory and parent directories
		// Similar to how OSS config loads it
		for _, path := range []string{".env", "../.env", "../../.env"} {
			if err := godotenv.Load(path); err == nil {
				log.Printf("Loaded .env file from %s", path)
				break
			}
		}
	}

	// Load database configuration from environment
	dbConfig, err := job.LoadConfigFromEnv()
	if err != nil {
		log.Printf("Warning: Failed to load database config from environment: %v. Using SQLite with default path.", err)
		// Fall back to SQLite with command-line path or default
		dbConfig = &job.DBConfig{
			Type:       "sqlite",
			SQLitePath: *dbPath,
		}
	}

	// Create job store based on configuration
	var jobStore job.Store
	if dbConfig.IsMySQLConfigured() {
		jobStore, err = job.NewStore(dbConfig)
		if err != nil {
			log.Fatalf("Failed to create MySQL job store: %v", err)
		}
		log.Printf("MySQL job store initialized (host: %s, port: %d, database: %s)", 
			dbConfig.MySQLHost, dbConfig.MySQLPort, dbConfig.MySQLDatabase)
	} else {
		// Use SQLite (either from config or fallback)
		if dbConfig.SQLitePath == "" {
			dbConfig.SQLitePath = *dbPath
		}
		jobStore, err = job.NewStore(dbConfig)
		if err != nil {
			log.Fatalf("Failed to create SQLite job store: %v", err)
		}
		log.Printf("SQLite job store initialized at: %s", dbConfig.SQLitePath)
	}
	defer func() {
		if err := jobStore.Close(); err != nil {
			log.Printf("Failed to close job store: %v", err)
		}
	}()

	// Initialize Redis queue (optional - queue can be nil if Redis is not configured)
	var jobQueue queue.Queue
	redisConfig := queue.LoadConfig()
	
	// Only initialize Redis if actually configured (not just using defaults)
	if redisConfig.IsConfigured() {
		redisClient, err := queue.NewRedisClient(redisConfig)
		if err != nil {
			log.Printf("Warning: Failed to connect to Redis: %v. Jobs will be created but not enqueued.", err)
			log.Printf("Redis is optional but recommended for job scheduling. Continuing without queue...")
		} else {
			defer func() {
				if err := redisClient.Close(); err != nil {
					log.Printf("Failed to close Redis client: %v", err)
				}
			}()
			jobQueue = queue.NewRedisQueue(redisClient)
			log.Printf("Redis queue initialized (host: %s, port: %d, db: %d)", redisConfig.Host, redisConfig.Port, redisConfig.Database)
		}
	} else {
		log.Printf("Redis not configured (REDIS_URL or REDIS_HOST not set). Jobs will be created but not enqueued.")
	}

	// Initialize OSS provider (required for job assignment)
	var ossProvider oss.Provider
	ossConfig, err := oss.LoadConfigFromEnv()
	if err != nil {
		log.Printf("Warning: Failed to load OSS config: %v. Job assignment will fail without OSS provider.", err)
		log.Printf("Set COS_SECRET_ID, COS_SECRET_KEY, COS_BUCKET, COS_REGION environment variables.")
	} else {
		ossProvider, err = oss.NewCOSProvider(ossConfig)
		if err != nil {
			log.Printf("Warning: Failed to create OSS provider: %v. Job assignment will fail without OSS provider.", err)
		} else {
			log.Printf("OSS provider initialized (bucket: %s, region: %s)", ossConfig.Bucket, ossConfig.Region)
		}
	}

	// Create registry
	reg := registry.New()

	// Create gateway with dependencies
	gw := gateway.New(reg, jobStore, jobQueue, ossProvider, *devMode)

	// Create API handler with queue
	apiHandler := api.New(reg, jobStore, jobQueue)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc(*wssPath, gw.HandleWebSocket)
	mux.HandleFunc("/api/agents/online", apiHandler.HandleAgentsOnline)
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			apiHandler.HandleCreateJob(w, r)
		case http.MethodGet:
			apiHandler.HandleListJobs(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiHandler.HandleGetJob(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/health", apiHandler.HandleHealth)

	// Start server
	server := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	go func() {
		log.Printf("Starting server on %s (dev mode: %v)", *addr, *devMode)
		if *devMode {
			log.Printf("WSS endpoint: ws://localhost%s%s", *addr, *wssPath)
		} else {
			log.Printf("WSS endpoint: wss://localhost%s%s", *addr, *wssPath)
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
}
