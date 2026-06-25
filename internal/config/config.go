package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	// Database
	DatabaseURL string

	// API Server
	APIPort int

	// Worker
	WorkerID       string
	Concurrency    int
	PollInterval   time.Duration
	LeaseTimeout   time.Duration
	ClaimBatchSize int

	// Defaults for tasks
	DefaultMaxAttempts       int
	DefaultTimeoutSeconds    int
	DefaultRetryDelaySeconds int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable"),
		APIPort:                  getEnvInt("API_PORT", 8080),
		WorkerID:                 getEnv("WORKER_ID", "worker-1"),
		Concurrency:              getEnvInt("WORKER_CONCURRENCY", 4),
		PollInterval:             getEnvDuration("WORKER_POLL_INTERVAL", 1*time.Second),
		LeaseTimeout:             getEnvDuration("LEASE_TIMEOUT", 90*time.Second),
		ClaimBatchSize:           getEnvInt("CLAIM_BATCH_SIZE", 5),
		DefaultMaxAttempts:       getEnvInt("DEFAULT_MAX_ATTEMPTS", 3),
		DefaultTimeoutSeconds:    getEnvInt("DEFAULT_TIMEOUT_SECONDS", 60),
		DefaultRetryDelaySeconds: getEnvInt("DEFAULT_RETRY_DELAY_SECONDS", 5),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
