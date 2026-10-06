package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config holds all configuration for the application.
type Config struct {
	// Profile is development by default. Production enables the strict
	// deployment safety checks in Validate.
	Profile string

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

	parseErrors []string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	c := &Config{
		Profile:     getEnv("APP_PROFILE", "development"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable"),
		WorkerID:    getEnv("WORKER_ID", defaultWorkerID()),
		APIToken:    os.Getenv("API_TOKEN"),
		AdminToken:  os.Getenv("ADMIN_TOKEN"),
		DocDir:      getEnv("RAG_DOC_DIR", "examples/docs"),
		ArtifactDir: getEnv("RAG_ARTIFACT_DIR", "artifacts/index"),
	}
	c.APIPort = c.int("API_PORT", 8080)
	c.WorkerHealthPort = c.int("WORKER_HEALTH_PORT", 9091)
	c.Concurrency = c.int("WORKER_CONCURRENCY", 4)
	c.PollInterval = c.duration("WORKER_POLL_INTERVAL", time.Second)
	c.LeaseTimeout = c.duration("LEASE_TIMEOUT", 90*time.Second)
	c.ClaimBatchSize = c.int("CLAIM_BATCH_SIZE", 5)
	c.ShutdownTimeout = c.duration("SHUTDOWN_TIMEOUT", 30*time.Second)
	c.DBOperationTimeout = c.duration("DB_OPERATION_TIMEOUT", 10*time.Second)
	c.DBMaxOpenConns = c.int("DB_MAX_OPEN_CONNS", 25)
	c.DBMaxIdleConns = c.int("DB_MAX_IDLE_CONNS", 5)
	c.DBConnMaxLifetime = c.duration("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	c.DBConnMaxIdleTime = c.duration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	c.MaxRequestBodyBytes = int64(c.int("MAX_REQUEST_BODY_BYTES", 1<<20))
	c.DefaultMaxAttempts = c.int("DEFAULT_MAX_ATTEMPTS", 3)
	c.DefaultTimeoutSeconds = c.int("DEFAULT_TIMEOUT_SECONDS", 60)
	c.DefaultRetryDelaySeconds = c.int("DEFAULT_RETRY_DELAY_SECONDS", 5)
	return c
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

func (c *Config) int(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			c.parseErrors = append(c.parseErrors, fmt.Sprintf("%s must be an integer: %q", key, v))
			return fallback
		}
		return n
	}
	return fallback
}

func (c *Config) duration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			c.parseErrors = append(c.parseErrors, fmt.Sprintf("%s must be a duration such as 30s: %q", key, v))
			return fallback
		}
		return d
	}
	return fallback
}

// Validate reports every invalid value at once. Call it before opening network
// connections so an operator never gets a process which silently chose a
// different safety setting than the one they configured.
func (c *Config) Validate() error {
	errs := append([]string(nil), c.parseErrors...)
	bad := func(ok bool, message string) {
		if !ok {
			errs = append(errs, message)
		}
	}
	bad(c.Profile == "development" || c.Profile == "production", "APP_PROFILE must be development or production")
	bad(c.APIPort > 0 && c.APIPort <= 65535, "API_PORT must be between 1 and 65535")
	bad(c.WorkerHealthPort > 0 && c.WorkerHealthPort <= 65535, "WORKER_HEALTH_PORT must be between 1 and 65535")
	bad(c.Concurrency > 0, "WORKER_CONCURRENCY must be greater than zero")
	bad(c.ClaimBatchSize > 0, "CLAIM_BATCH_SIZE must be greater than zero")
	bad(c.PollInterval > 0, "WORKER_POLL_INTERVAL must be greater than zero")
	bad(c.LeaseTimeout > 0, "LEASE_TIMEOUT must be greater than zero")
	bad(c.DBOperationTimeout > 0, "DB_OPERATION_TIMEOUT must be greater than zero")
	bad(c.DBOperationTimeout < c.LeaseTimeout, "DB_OPERATION_TIMEOUT must be less than LEASE_TIMEOUT")
	bad(c.ShutdownTimeout > 0, "SHUTDOWN_TIMEOUT must be greater than zero")
	bad(c.DBMaxOpenConns > 0, "DB_MAX_OPEN_CONNS must be greater than zero")
	bad(c.DBMaxIdleConns >= 0 && c.DBMaxIdleConns <= c.DBMaxOpenConns, "DB_MAX_IDLE_CONNS must be between 0 and DB_MAX_OPEN_CONNS")
	bad(c.Concurrency <= c.DBMaxOpenConns, "WORKER_CONCURRENCY must not exceed DB_MAX_OPEN_CONNS")
	bad(c.MaxRequestBodyBytes > 0, "MAX_REQUEST_BODY_BYTES must be greater than zero")
	bad(c.DefaultMaxAttempts > 0, "DEFAULT_MAX_ATTEMPTS must be greater than zero")
	bad(c.DefaultTimeoutSeconds > 0, "DEFAULT_TIMEOUT_SECONDS must be greater than zero")
	bad(c.DefaultRetryDelaySeconds >= 0 && c.DefaultRetryDelaySeconds < c.DefaultTimeoutSeconds, "DEFAULT_RETRY_DELAY_SECONDS must be non-negative and less than DEFAULT_TIMEOUT_SECONDS")

	u, urlErr := url.Parse(c.DatabaseURL)
	bad(urlErr == nil && u.Scheme == "postgres" || urlErr == nil && u.Scheme == "postgresql", "DATABASE_URL must be a PostgreSQL URL")
	if c.Profile == "production" {
		bad(c.APIToken != "", "API_TOKEN is required in production")
		bad(c.AdminToken != "", "ADMIN_TOKEN is required in production")
		bad(c.APIToken == "" || c.AdminToken == "" || c.APIToken != c.AdminToken, "API_TOKEN and ADMIN_TOKEN must be distinct in production")
		if urlErr == nil {
			bad(u.Query().Get("sslmode") != "disable", "DATABASE_URL must not use sslmode=disable in production")
			if password, set := u.User.Password(); set {
				bad(len(password) >= 16 && !strings.EqualFold(password, "postgres"), "DATABASE_URL password is too weak for production")
			} else {
				errs = append(errs, "DATABASE_URL must include a password in production")
			}
		}
		bad(!isLocalArtifactDir(c.ArtifactDir), "RAG_ARTIFACT_DIR must reference shared object storage in production, not a local directory")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n - %s", strings.Join(errs, "\n - "))
}

func isLocalArtifactDir(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "" || !(strings.HasPrefix(v, "s3://") || strings.HasPrefix(v, "minio://"))
}

// EffectiveChecksum returns a stable fingerprint of non-secret settings for
// correlation with logs and deployments. Tokens and database credentials are
// intentionally excluded.
func (c *Config) EffectiveChecksum() string {
	value := strings.Join([]string{c.Profile, c.DatabaseURLWithoutCredentials(), strconv.Itoa(c.APIPort), strconv.Itoa(c.Concurrency), c.PollInterval.String(), c.LeaseTimeout.String(), c.ArtifactDir}, "|")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (c *Config) DatabaseURLWithoutCredentials() string {
	u, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return "invalid"
	}
	u.User = nil
	return u.String()
}
