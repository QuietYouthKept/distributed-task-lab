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
	CursorSigningKey    string

	// OIDC is the production authentication boundary. Static bearer tokens are
	// retained only for explicitly local development.
	OIDCIssuer           string
	OIDCAudience         string
	OIDCJWKSURL          string
	OIDCRoleClaim        string
	OIDCRoleMappings     map[string]string
	OIDCServiceClientIDs map[string]struct{}

	// Defaults for tasks
	DefaultMaxAttempts       int
	DefaultTimeoutSeconds    int
	DefaultRetryDelaySeconds int

	// RAG ingestion (real lightweight workload)
	DocDir                    string
	ArtifactDir               string
	ArtifactBackend           string
	ArtifactS3Endpoint        string
	ArtifactS3Bucket          string
	ArtifactS3Prefix          string
	ArtifactS3AccessKey       string
	ArtifactS3SecretKey       string
	ArtifactS3Region          string
	ArtifactMaxObjectBytes    int64
	ArtifactAllowHTTP         bool
	ArtifactAllowBucketCreate bool

	// Eval provider selection. Deterministic is development scaffolding;
	// production requires the explicitly configured gateway backend.
	EvalBackend        string
	EvalGatewayURL     string
	EvalGatewayProfile string
	EvalGatewayAPIKey  string

	parseErrors []string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	c := &Config{
		Profile:              getEnv("APP_PROFILE", "development"),
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable"),
		WorkerID:             getEnv("WORKER_ID", defaultWorkerID()),
		APIToken:             os.Getenv("API_TOKEN"),
		AdminToken:           os.Getenv("ADMIN_TOKEN"),
		OIDCIssuer:           os.Getenv("OIDC_ISSUER"),
		OIDCAudience:         os.Getenv("OIDC_AUDIENCE"),
		OIDCJWKSURL:          os.Getenv("OIDC_JWKS_URL"),
		OIDCRoleClaim:        getEnv("OIDC_ROLE_CLAIM", "roles"),
		OIDCRoleMappings:     parseRoleMappings(os.Getenv("OIDC_ROLE_MAPPINGS")),
		OIDCServiceClientIDs: parseSet(os.Getenv("OIDC_SERVICE_CLIENT_IDS")),
		DocDir:               getEnv("RAG_DOC_DIR", "examples/docs"),
		ArtifactDir:          getEnv("RAG_ARTIFACT_DIR", "artifacts/index"),
		ArtifactBackend:      getEnv("ARTIFACT_STORE_BACKEND", "local"),
		ArtifactS3Endpoint:   os.Getenv("ARTIFACT_S3_ENDPOINT"),
		ArtifactS3Bucket:     os.Getenv("ARTIFACT_S3_BUCKET"),
		ArtifactS3Prefix:     os.Getenv("ARTIFACT_S3_PREFIX"),
		ArtifactS3AccessKey:  os.Getenv("ARTIFACT_S3_ACCESS_KEY"),
		ArtifactS3SecretKey:  os.Getenv("ARTIFACT_S3_SECRET_KEY"),
		ArtifactS3Region:     getEnv("ARTIFACT_S3_REGION", "us-east-1"),
		CursorSigningKey:     os.Getenv("API_CURSOR_SIGNING_KEY"),
		EvalBackend:          getEnv("EVAL_BACKEND", "deterministic"),
		EvalGatewayURL:       os.Getenv("EVAL_GATEWAY_URL"),
		EvalGatewayProfile:   os.Getenv("EVAL_GATEWAY_PROFILE"),
		EvalGatewayAPIKey:    os.Getenv("EVAL_GATEWAY_API_KEY"),
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
	c.ArtifactMaxObjectBytes = int64(c.int("ARTIFACT_MAX_OBJECT_BYTES", 64<<20))
	c.ArtifactAllowHTTP = c.boolean("ARTIFACT_ALLOW_HTTP", false)
	c.ArtifactAllowBucketCreate = c.boolean("ARTIFACT_ALLOW_BUCKET_CREATE", false)
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

func (c *Config) boolean(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			c.parseErrors = append(c.parseErrors, fmt.Sprintf("%s must be a boolean: %q", key, value))
			return fallback
		}
		return parsed
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
	bad(c.EvalBackend == "deterministic" || c.EvalBackend == "gateway", "EVAL_BACKEND must be deterministic or gateway")
	bad(c.ArtifactBackend == "local" || c.ArtifactBackend == "s3", "ARTIFACT_STORE_BACKEND must be local or s3")
	bad(c.ArtifactMaxObjectBytes > 0 && c.ArtifactMaxObjectBytes <= 5<<30, "ARTIFACT_MAX_OBJECT_BYTES must be between 1 byte and 5 GiB")
	if c.ArtifactBackend == "s3" {
		endpoint, endpointErr := url.Parse(c.ArtifactS3Endpoint)
		bad(endpointErr == nil && endpoint.Host != "" && endpoint.User == nil && endpoint.Path == "" && endpoint.RawQuery == "" && endpoint.Fragment == "", "ARTIFACT_S3_ENDPOINT must be an absolute URL without path, userinfo, query, or fragment")
		bad(c.ArtifactS3Bucket != "" && strings.TrimSpace(c.ArtifactS3Bucket) == c.ArtifactS3Bucket && !strings.ContainsAny(c.ArtifactS3Bucket, "/\\:%"), "ARTIFACT_S3_BUCKET is invalid")
		bad(c.ArtifactS3AccessKey != "" && c.ArtifactS3SecretKey != "", "ARTIFACT_S3_ACCESS_KEY and ARTIFACT_S3_SECRET_KEY are required for S3")
	}
	if c.EvalBackend == "gateway" {
		gatewayURL, gatewayErr := url.Parse(c.EvalGatewayURL)
		bad(gatewayErr == nil && gatewayURL.Host != "" && (gatewayURL.Scheme == "http" || gatewayURL.Scheme == "https"), "EVAL_GATEWAY_URL must be an absolute HTTP(S) URL for gateway backend")
		bad(strings.TrimSpace(c.EvalGatewayProfile) != "", "EVAL_GATEWAY_PROFILE is required for gateway backend")
	}

	u, urlErr := url.Parse(c.DatabaseURL)
	bad(urlErr == nil && u.Scheme == "postgres" || urlErr == nil && u.Scheme == "postgresql", "DATABASE_URL must be a PostgreSQL URL")
	if c.Profile == "production" {
		bad(len(c.CursorSigningKey) >= 32, "API_CURSOR_SIGNING_KEY must contain at least 32 bytes in production")
		issuer, issuerErr := url.Parse(c.OIDCIssuer)
		jwks, jwksErr := url.Parse(c.OIDCJWKSURL)
		bad(issuerErr == nil && issuer.Scheme == "https" && issuer.Host != "", "OIDC_ISSUER must be an absolute HTTPS URL in production")
		bad(strings.TrimSpace(c.OIDCAudience) != "", "OIDC_AUDIENCE is required in production")
		bad(jwksErr == nil && jwks.Scheme == "https" && jwks.Host != "", "OIDC_JWKS_URL must be an absolute HTTPS URL in production")
		bad(c.OIDCRoleClaim == "roles" || c.OIDCRoleClaim == "groups", "OIDC_ROLE_CLAIM must be roles or groups")
		bad(len(c.OIDCRoleMappings) > 0, "OIDC_ROLE_MAPPINGS must explicitly map trusted claims to roles in production")
		bad(c.APIToken == "" && c.AdminToken == "", "API_TOKEN and ADMIN_TOKEN static authentication are forbidden in production")
		if urlErr == nil {
			bad(u.Query().Get("sslmode") != "disable", "DATABASE_URL must not use sslmode=disable in production")
			bad(!isLocalDatabaseHost(u.Hostname()), "DATABASE_URL must not target a local database host in production")
			if password, set := u.User.Password(); set {
				bad(len(password) >= 16 && !strings.EqualFold(password, "postgres"), "DATABASE_URL password is too weak for production")
			} else {
				errs = append(errs, "DATABASE_URL must include a password in production")
			}
		}
		bad(c.ArtifactBackend == "s3", "ARTIFACT_STORE_BACKEND must be s3 in production")
		artifactEndpoint, artifactErr := url.Parse(c.ArtifactS3Endpoint)
		bad(artifactErr == nil && artifactEndpoint.Scheme == "https" && artifactEndpoint.Host != "" && artifactEndpoint.User == nil && artifactEndpoint.Path == "" && artifactEndpoint.RawQuery == "" && artifactEndpoint.Fragment == "", "ARTIFACT_S3_ENDPOINT must use HTTPS and contain no path or credentials in production")
		bad(c.ArtifactS3Bucket != "" && c.ArtifactS3AccessKey != "" && c.ArtifactS3SecretKey != "", "S3 bucket and credentials are required in production")
		bad(!c.ArtifactAllowHTTP && !c.ArtifactAllowBucketCreate, "HTTP endpoints and automatic bucket creation are forbidden in production")
		bad(c.EvalBackend == "gateway", "EVAL_BACKEND must be gateway in production")
		gatewayURL, gatewayErr := url.Parse(c.EvalGatewayURL)
		bad(gatewayErr == nil && gatewayURL.Scheme == "https" && gatewayURL.Host != "", "EVAL_GATEWAY_URL must use HTTPS in production")
		bad(strings.TrimSpace(c.EvalGatewayAPIKey) != "", "EVAL_GATEWAY_API_KEY is required in production")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n - %s", strings.Join(errs, "\n - "))
}

func parseRoleMappings(raw string) map[string]string {
	result := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 2)
		if len(parts) == 2 && parts[0] != "" && (parts[1] == "reader" || parts[1] == "operator" || parts[1] == "admin") {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func parseSet(raw string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

// ValidateWorker adds worker-only deployment checks. Shared artifact storage
// is wired for explicitly configured S3 deployments, but no execution type has
// completed the production acceptance gate. Production workers therefore
// remain fail-closed until the execution policy is explicitly approved.
func (c *Config) ValidateWorker() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Profile == "production" {
		return fmt.Errorf("invalid worker configuration: production execution allowlist is empty; refusing to start before a task type passes the production acceptance gate")
	}
	return nil
}

func isLocalDatabaseHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "postgres"
}

func isLocalArtifactDir(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "" || !(strings.HasPrefix(v, "s3://") || strings.HasPrefix(v, "minio://"))
}

// EffectiveChecksum returns a stable fingerprint of non-secret settings for
// correlation with logs and deployments. Tokens and database credentials are
// intentionally excluded.
func (c *Config) EffectiveChecksum() string {
	value := strings.Join([]string{c.Profile, c.DatabaseURLWithoutCredentials(), strconv.Itoa(c.APIPort), strconv.Itoa(c.Concurrency), c.PollInterval.String(), c.LeaseTimeout.String(), c.ArtifactBackend, c.ArtifactS3Endpoint, c.ArtifactS3Bucket, c.ArtifactS3Prefix, strconv.FormatInt(c.ArtifactMaxObjectBytes, 10), c.ArtifactDir, c.EvalBackend, c.EvalGatewayURL, c.EvalGatewayProfile}, "|")
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
