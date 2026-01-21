package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xiresource/agent/internal/client"
)

func main() {
	var (
		serverURL      = flag.String("server", "ws://localhost:8080/wss", "Server WebSocket URL")
		agentID        = flag.String("agent-id", "", "Agent ID (required)")
		agentToken     = flag.String("agent-token", "dev-token", "Agent token (dev mode)")
		maxConcurrency = flag.Int("max-concurrency", 1, "Maximum concurrent jobs")
		inputCacheTTL  = flag.Duration("input-cache-ttl", 10*time.Minute, "Input cache TTL for forward jobs (0 to disable)")
	)
	flag.Parse()

	if *agentID == "" {
		log.Fatal("agent-id is required")
	}

	// Create client
	cli := client.New(*serverURL, *agentID, *agentToken, *maxConcurrency)
	cli.SetInputCacheTTL(*inputCacheTTL)

	// Connect
	if err := cli.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Keep running
	<-sigChan

	log.Println("Shutting down agent...")
	cli.Stop()
	time.Sleep(100 * time.Millisecond) // Give time for cleanup
}
