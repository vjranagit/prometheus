package config

import (
	"fmt"
	"os"
	"time"

	"github.com/vjranagit/prometheus/pkg/storage"
)

// Config holds the application configuration
type Config struct {
	Server  ServerConfig  `json:"server"`
	Storage StorageConfig `json:"storage"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	ListenAddr string        `json:"listen_addr"`
	Timeout    time.Duration `json:"timeout"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	Path             string `json:"path"`
	RetentionDays    int    `json:"retention_days"`
	CompressionLevel int    `json:"compression_level"`
	MaxOpenFiles     int    `json:"max_open_files"`
	EnableWAL        bool   `json:"enable_wal"`
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenAddr: ":9090",
			Timeout:    30 * time.Second,
		},
		Storage: StorageConfig{
			Path:             getEnv("STORAGE_PATH", "./data"),
			RetentionDays:    getEnvInt("RETENTION_DAYS", 30),
			CompressionLevel: getEnvInt("COMPRESSION_LEVEL", 3),
			MaxOpenFiles:     getEnvInt("MAX_OPEN_FILES", 1000),
			EnableWAL:        getEnvBool("ENABLE_WAL", true),
		},
	}
}

// ToStorageConfig converts to storage.Config
func (c *Config) ToStorageConfig() *storage.Config {
	return &storage.Config{
		Path:             c.Storage.Path,
		RetentionDays:    c.Storage.RetentionDays,
		CompressionLevel: c.Storage.CompressionLevel,
		MaxOpenFiles:     c.Storage.MaxOpenFiles,
		EnableWAL:        c.Storage.EnableWAL,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server listen address is required")
	}

	if c.Storage.Path == "" {
		return fmt.Errorf("storage path is required")
	}

	if c.Storage.RetentionDays < 1 {
		return fmt.Errorf("retention days must be at least 1")
	}

	if c.Storage.CompressionLevel < 1 || c.Storage.CompressionLevel > 4 {
		return fmt.Errorf("compression level must be between 1 and 4")
	}

	return nil
}

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}
