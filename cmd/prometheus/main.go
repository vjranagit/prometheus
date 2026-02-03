package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vjranagit/prometheus/internal/config"
	"github.com/vjranagit/prometheus/pkg/api"
	"github.com/vjranagit/prometheus/pkg/storage"
)

const (
	version = "0.2.0"
)

func main() {
	fmt.Printf("Prometheus Fork v%s\n", version)
	fmt.Println("High-performance time-series database")
	fmt.Println()

	// Load configuration
	cfg := config.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  Listen Address: %s", cfg.Server.ListenAddr)
	log.Printf("  Storage Path: %s", cfg.Storage.Path)
	log.Printf("  Retention: %d days", cfg.Storage.RetentionDays)
	log.Printf("  Compression Level: %d", cfg.Storage.CompressionLevel)

	// Initialize storage
	log.Println("Initializing storage engine...")
	store, err := storage.NewStorage(cfg.ToStorageConfig())
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	log.Println("Storage engine initialized")

	// Create API server
	log.Println("Starting API server...")
	server := api.NewServer(cfg.Server.ListenAddr, store)

	// Start server in goroutine
	go func() {
		log.Printf("API server listening on %s", cfg.Server.ListenAddr)
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutdown signal received, stopping server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped successfully")
}
