package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Config holds all configuration for the application.
type Config struct {
	// Database
	DatabaseURL string

	// API Server
	APIPort int

	// Worker
	WorkerID           string
	WorkerHealthPort   int
	Concurrency        int
	PollInterval       time.Duration
	LeaseTimeout       time.Duration
	ClaimBatchSize     int
	ShutdownTimeout    time.Duration
	DBOperationTimeout time.Duration
	DBMaxOpenConns     int
	DBMaxIdleConns     int
	DBConnMaxLifetime  time.Duration
	DBConnMaxIdleTime  time.Duration

	// Optional HTTP bearer tokens. When unset the corresponding surface is
	// intentionally unauthenticated for local development.
	APIToken            string
	AdminToken          string
	MaxRequestBodyBytes int64

	// Defaults for tasks
	DefaultMaxAttempts       int
	DefaultTimeoutSeconds    int
	DefaultRetryDelaySeconds int

	// RAG ingestion (real lightweight workload)
	DocDir      string
	ArtifactDir string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable"),
		APIPort:                  getEnvInt("API_PORT", 8080),
		WorkerID:                 getEnv("WORKER_ID", defaultWorkerID()),
		WorkerHealthPort:         getEnvInt("WORKER_HEALTH_PORT", 9091),
		Concurrency:              getEnvInt("WORKER_CONCURRENCY", 4),
		PollInterval:             getEnvDuration("WORKER_POLL_INTERVAL", 1*time.Second),
		LeaseTimeout:             getEnvDuration("LEASE_TIMEOUT", 90*time.Second),
		ClaimBatchSize:           getEnvInt("CLAIM_BATCH_SIZE", 5),
		ShutdownTimeout:          getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		DBOperationTimeout:       getEnvDuration("DB_OPERATION_TIMEOUT", 10*time.Second),
		DBMaxOpenConns:           getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:           getEnvInt("DB_MAX_IDLE_CONNS", 5),
		DBConnMaxLifetime:        getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		DBConnMaxIdleTime:        getEnvDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		APIToken:                 os.Getenv("API_TOKEN"),
		AdminToken:               os.Getenv("ADMIN_TOKEN"),
		MaxRequestBodyBytes:      int64(getEnvInt("MAX_REQUEST_BODY_BYTES", 1<<20)),
		DefaultMaxAttempts:       getEnvInt("DEFAULT_MAX_ATTEMPTS", 3),
		DefaultTimeoutSeconds:    getEnvInt("DEFAULT_TIMEOUT_SECONDS", 60),
		DefaultRetryDelaySeconds: getEnvInt("DEFAULT_RETRY_DELAY_SECONDS", 5),
		DocDir:                   getEnv("RAG_DOC_DIR", "examples/docs"),
		ArtifactDir:              getEnv("RAG_ARTIFACT_DIR", "artifacts/index"),
	}
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString())
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
