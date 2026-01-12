package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/xiresource/cloud/internal/oss"
)

func main() {
	// Load config from environment
	config, err := oss.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("Failed to load config: %v\n\nPlease set the following environment variables:\n"+
			"  COS_SECRET_ID\n  COS_SECRET_KEY\n  COS_BUCKET\n  COS_REGION\n"+
			"Optional: COS_PRESIGN_TTL_MINUTES (default: 15)", err)
	}

	// Create provider
	provider, err := oss.NewCOSProvider(config)
	if err != nil {
		log.Fatalf("Failed to create COS provider: %v", err)
	}

	ctx := context.Background()

	// Test 1: Generate download URL
	fmt.Println("=== Test 1: Generate Download URL ===")
	testKey := "car_image/163com.jpg"
	if len(os.Args) > 1 {
		testKey = os.Args[1]
	}

	downloadURL, err := provider.GenerateDownloadURL(ctx, testKey)
	if err != nil {
		log.Fatalf("Failed to generate download URL: %v", err)
	}
	fmt.Printf("Key: %s\n", testKey)
	fmt.Printf("Download URL: %s\n", downloadURL)
	fmt.Printf("\nTo test with curl (PowerShell):\n")
	fmt.Printf("  Invoke-WebRequest -Uri \"%s\" -Method HEAD -UseBasicParsing\n", downloadURL)
	fmt.Printf("\nOr with curl.exe (Windows):\n")
	fmt.Printf("  curl.exe -I \"%s\"\n\n", downloadURL)

	// Test 2: Generate upload URL
	fmt.Println("=== Test 2: Generate Upload URL ===")
	uploadKey := "zfc_files/test/cursor_test.txt"
	if len(os.Args) > 2 {
		uploadKey = os.Args[2]
	}

	uploadURL, err := provider.GenerateUploadURL(ctx, uploadKey)
	if err != nil {
		log.Fatalf("Failed to generate upload URL: %v", err)
	}
	fmt.Printf("Key: %s\n", uploadKey)
	fmt.Printf("Upload URL: %s\n", uploadURL)
	fmt.Printf("\nTo test with curl (PowerShell):\n")
	fmt.Printf("  $body = 'test content'; Invoke-WebRequest -Uri \"%s\" -Method PUT -Body $body -UseBasicParsing\n", uploadURL)

	// Test 3: Generate upload URL with prefix
	fmt.Println("=== Test 3: Generate Upload URL with Prefix ===")
	prefix := "jobs/job-123/attempt-1"
	filename := "output.zip"
	if len(os.Args) > 3 {
		prefix = os.Args[3]
	}
	if len(os.Args) > 4 {
		filename = os.Args[4]
	}

	uploadURLWithPrefix, err := provider.GenerateUploadURLWithPrefix(ctx, prefix, filename)
	if err != nil {
		log.Fatalf("Failed to generate upload URL with prefix: %v", err)
	}
	fmt.Printf("Prefix: %s/\n", prefix)
	fmt.Printf("Filename: %s\n", filename)
	fmt.Printf("Full Key: %s/%s\n", prefix, filename)
	fmt.Printf("Upload URL: %s\n", uploadURLWithPrefix)
	fmt.Printf("\nTo test with curl (PowerShell):\n")
	fmt.Printf("  $body = 'test content'; Invoke-WebRequest -Uri \"%s\" -Method PUT -Body $body -UseBasicParsing\n", uploadURLWithPrefix)

	fmt.Println("=== All tests passed! ===")
}
