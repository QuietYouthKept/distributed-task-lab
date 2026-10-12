package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
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
	AutoMigrate bool

	// API Server
	APIPort        int
	APITLSCertFile string
	APITLSKeyFile  string

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
	APIToken                  string
	AdminToken                string
	MaxRequestBodyBytes       int64
	CursorSigningKey          string
	CursorRotationEnabled     bool
	CursorActiveKeyID         string
	CursorActiveKey           string
	CursorTTL                 time.Duration
	CursorPreviousKeys        []CursorPreviousKey
	CursorPreviousKeysRaw     string
	CursorLegacyV1Key         string
	CursorLegacyV1AcceptUntil time.Time

	// Shared API admission is a Wave 4 candidate and remains disabled unless
	// every policy and key setting is explicitly supplied.
	SharedRateLimitEnabled     bool
	SharedRateLimitRate        float64
	SharedRateLimitBurst       int
	SharedRateLimitFailureMode string
	SharedRateLimitHashKey     string

	// OIDC is the production authentication boundary. Static bearer tokens are
	// retained only for explicitly local development.
	OIDCIssuer           string
	OIDCAudience         string
	OIDCJWKSURL          string
	OIDCCAFile           string
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
	ArtifactOSSEndpoint       string
	ArtifactOSSRegion         string
	ArtifactOSSBucket         string
	ArtifactOSSPrefix         string
	ArtifactOSSRAMRoleName    string
	StagingSimCloud           bool
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

type CursorPreviousKey struct {
	ID             string    `json:"id"`
	Key            string    `json:"key"`
	AcceptUntil    time.Time `json:"-"`
	AcceptUntilRaw string    `json:"accept_until"`
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	c := &Config{
		Profile:                getEnv("APP_PROFILE", "development"),
		DatabaseURL:            getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable"),
		AutoMigrate:            true,
		WorkerID:               getEnv("WORKER_ID", defaultWorkerID()),
		APIToken:               os.Getenv("API_TOKEN"),
		AdminToken:             os.Getenv("ADMIN_TOKEN"),
		APITLSCertFile:         os.Getenv("API_TLS_CERT_FILE"),
		APITLSKeyFile:          os.Getenv("API_TLS_KEY_FILE"),
		OIDCIssuer:             os.Getenv("OIDC_ISSUER"),
		OIDCAudience:           os.Getenv("OIDC_AUDIENCE"),
		OIDCJWKSURL:            os.Getenv("OIDC_JWKS_URL"),
		OIDCCAFile:             os.Getenv("OIDC_CA_CERT_FILE"),
		OIDCRoleClaim:          getEnv("OIDC_ROLE_CLAIM", "roles"),
		OIDCRoleMappings:       parseRoleMappings(os.Getenv("OIDC_ROLE_MAPPINGS")),
		OIDCServiceClientIDs:   parseSet(os.Getenv("OIDC_SERVICE_CLIENT_IDS")),
		DocDir:                 getEnv("RAG_DOC_DIR", "examples/docs"),
		ArtifactDir:            getEnv("RAG_ARTIFACT_DIR", "artifacts/index"),
		ArtifactBackend:        getEnv("ARTIFACT_STORE_BACKEND", "local"),
		ArtifactS3Endpoint:     os.Getenv("ARTIFACT_S3_ENDPOINT"),
		ArtifactS3Bucket:       os.Getenv("ARTIFACT_S3_BUCKET"),
		ArtifactS3Prefix:       os.Getenv("ARTIFACT_S3_PREFIX"),
		ArtifactS3AccessKey:    os.Getenv("ARTIFACT_S3_ACCESS_KEY"),
		ArtifactS3SecretKey:    os.Getenv("ARTIFACT_S3_SECRET_KEY"),
		ArtifactS3Region:       getEnv("ARTIFACT_S3_REGION", "us-east-1"),
		ArtifactOSSEndpoint:    os.Getenv("ARTIFACT_OSS_ENDPOINT"),
		ArtifactOSSRegion:      os.Getenv("ARTIFACT_OSS_REGION"),
		ArtifactOSSBucket:      os.Getenv("ARTIFACT_OSS_BUCKET"),
		ArtifactOSSPrefix:      os.Getenv("ARTIFACT_OSS_PREFIX"),
		ArtifactOSSRAMRoleName: os.Getenv("ARTIFACT_OSS_RAM_ROLE_NAME"),
		CursorSigningKey:       os.Getenv("API_CURSOR_SIGNING_KEY"),
		EvalBackend:            getEnv("EVAL_BACKEND", "deterministic"),
		EvalGatewayURL:         os.Getenv("EVAL_GATEWAY_URL"),
		EvalGatewayProfile:     os.Getenv("EVAL_GATEWAY_PROFILE"),
		EvalGatewayAPIKey:      os.Getenv("EVAL_GATEWAY_API_KEY"),
	}
	c.StagingSimCloud = c.boolean("STAGING_SIM_CLOUD", false)
	for name, target := range map[string]*string{
		"DATABASE_URL":           &c.DatabaseURL,
		"API_TOKEN":              &c.APIToken,
		"ADMIN_TOKEN":            &c.AdminToken,
		"API_CURSOR_SIGNING_KEY": &c.CursorSigningKey,
		"ARTIFACT_S3_ACCESS_KEY": &c.ArtifactS3AccessKey,
		"ARTIFACT_S3_SECRET_KEY": &c.ArtifactS3SecretKey,
		"EVAL_GATEWAY_API_KEY":   &c.EvalGatewayAPIKey,
	} {
		*target = c.secretEnv(name, *target)
	}
	c.AutoMigrate = c.boolean("AUTO_MIGRATE", true)
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
	c.SharedRateLimitEnabled = c.boolean("API_SHARED_RATE_LIMIT_ENABLED", false)
	c.SharedRateLimitRate = c.float("API_SHARED_RATE_LIMIT_RATE_PER_SECOND", 0)
	c.SharedRateLimitBurst = c.int("API_SHARED_RATE_LIMIT_BURST", 0)
	c.SharedRateLimitFailureMode = os.Getenv("API_SHARED_RATE_LIMIT_FAILURE_MODE")
	c.SharedRateLimitHashKey = c.secretEnv("API_SHARED_RATE_LIMIT_HASH_KEY", os.Getenv("API_SHARED_RATE_LIMIT_HASH_KEY"))
	c.CursorRotationEnabled = c.boolean("API_CURSOR_KEY_RING_ENABLED", false)
	c.CursorActiveKeyID = os.Getenv("API_CURSOR_ACTIVE_KEY_ID")
	c.CursorActiveKey = c.secretEnv("API_CURSOR_ACTIVE_KEY", os.Getenv("API_CURSOR_ACTIVE_KEY"))
	c.CursorTTL = c.duration("API_CURSOR_TTL", 0)
	c.CursorPreviousKeysRaw = c.secretEnv("API_CURSOR_PREVIOUS_KEYS", os.Getenv("API_CURSOR_PREVIOUS_KEYS"))
	c.CursorLegacyV1Key = c.secretEnv("API_CURSOR_V1_LEGACY_KEY", os.Getenv("API_CURSOR_V1_LEGACY_KEY"))
	if raw := os.Getenv("API_CURSOR_V1_ACCEPT_UNTIL"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.parseErrors = append(c.parseErrors, "API_CURSOR_V1_ACCEPT_UNTIL must be RFC3339")
		} else {
			c.CursorLegacyV1AcceptUntil = parsed
		}
	}
	if c.CursorPreviousKeysRaw != "" {
		var previous []CursorPreviousKey
		if err := json.Unmarshal([]byte(c.CursorPreviousKeysRaw), &previous); err != nil {
			c.parseErrors = append(c.parseErrors, "API_CURSOR_PREVIOUS_KEYS must be a JSON array of key entries")
		} else {
			for i := range previous {
				parsed, err := time.Parse(time.RFC3339, previous[i].AcceptUntilRaw)
				if err != nil {
					c.parseErrors = append(c.parseErrors, "API_CURSOR_PREVIOUS_KEYS accept_until must be RFC3339")
				} else {
					previous[i].AcceptUntil = parsed
				}
			}
			c.CursorPreviousKeys = previous
		}
	}
	return c
}

// secretEnv supports mounted secret files while retaining the environment
// variable interface for existing deployments. It deliberately avoids adding
// file contents or paths to validation errors.
func (c *Config) secretEnv(name, fallback string) string {
	direct, file := os.Getenv(name), os.Getenv(name+"_FILE")
	if direct != "" && file != "" {
		c.parseErrors = append(c.parseErrors, name+" and "+name+"_FILE cannot both be set")
		return ""
	}
	if file == "" {
		if direct != "" {
			return direct
		}
		return fallback
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		c.parseErrors = append(c.parseErrors, name+"_FILE could not be read")
		return ""
	}
	value := strings.TrimRight(string(contents), "\r\n")
	if value == "" {
		c.parseErrors = append(c.parseErrors, name+"_FILE must not be empty")
		return ""
	}
	return value
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

func (c *Config) float(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			c.parseErrors = append(c.parseErrors, fmt.Sprintf("%s must be a number: %q", key, v))
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
	bad(c.Profile == "development" || c.Profile == "production" || c.Profile == "staging-sim", "APP_PROFILE must be development, staging-sim, or production")
	bad(!c.StagingSimCloud || c.Profile == "staging-sim", "STAGING_SIM_CLOUD is permitted only with APP_PROFILE=staging-sim")
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
	if c.SharedRateLimitEnabled {
		bad(c.SharedRateLimitRate >= 0.001 && c.SharedRateLimitRate <= 10000 && !math.IsNaN(c.SharedRateLimitRate) && !math.IsInf(c.SharedRateLimitRate, 0), "API_SHARED_RATE_LIMIT_RATE_PER_SECOND must be between 0.001 and 10000 when shared rate limiting is enabled")
		bad(c.SharedRateLimitBurst > 0 && c.SharedRateLimitBurst <= 1000000, "API_SHARED_RATE_LIMIT_BURST must be between 1 and 1000000 when shared rate limiting is enabled")
		bad(c.SharedRateLimitFailureMode == "open" || c.SharedRateLimitFailureMode == "closed", "API_SHARED_RATE_LIMIT_FAILURE_MODE must be explicitly set to open or closed when shared rate limiting is enabled")
		bad(len(c.SharedRateLimitHashKey) >= 32, "API_SHARED_RATE_LIMIT_HASH_KEY must contain at least 32 bytes when shared rate limiting is enabled")
	}
	if c.CursorRotationEnabled {
		bad(strings.TrimSpace(c.CursorActiveKeyID) != "", "API_CURSOR_ACTIVE_KEY_ID is required when cursor rotation is enabled")
		bad(len(c.CursorActiveKey) >= 32, "API_CURSOR_ACTIVE_KEY must contain at least 32 bytes when cursor rotation is enabled")
		bad(c.CursorTTL >= time.Second && c.CursorTTL <= 7*24*time.Hour, "API_CURSOR_TTL must be between 1s and 168h when cursor rotation is enabled")
		seenKeyIDs := map[string]struct{}{c.CursorActiveKeyID: {}}
		for _, previous := range c.CursorPreviousKeys {
			_, duplicate := seenKeyIDs[previous.ID]
			bad(strings.TrimSpace(previous.ID) != "" && !duplicate, "API_CURSOR_PREVIOUS_KEYS must have unique IDs distinct from the active key")
			bad(len(previous.Key) >= 32 && !previous.AcceptUntil.IsZero(), "each previous cursor key requires at least 32 key bytes and an accept_until timestamp")
			seenKeyIDs[previous.ID] = struct{}{}
		}
		if c.CursorLegacyV1Key != "" {
			bad(len(c.CursorLegacyV1Key) >= 32 && !c.CursorLegacyV1AcceptUntil.IsZero(), "API_CURSOR_V1_LEGACY_KEY requires at least 32 bytes and API_CURSOR_V1_ACCEPT_UNTIL")
		} else {
			bad(c.CursorLegacyV1AcceptUntil.IsZero(), "API_CURSOR_V1_ACCEPT_UNTIL requires API_CURSOR_V1_LEGACY_KEY")
		}
	}
	bad(c.EvalBackend == "deterministic" || c.EvalBackend == "gateway", "EVAL_BACKEND must be deterministic or gateway")
	bad(c.ArtifactBackend == "local" || c.ArtifactBackend == "s3" || c.ArtifactBackend == "oss", "ARTIFACT_STORE_BACKEND must be local, s3, or oss")
	bad(c.ArtifactMaxObjectBytes > 0 && c.ArtifactMaxObjectBytes <= 5<<30, "ARTIFACT_MAX_OBJECT_BYTES must be between 1 byte and 5 GiB")
	if c.ArtifactBackend == "s3" {
		endpoint, endpointErr := url.Parse(c.ArtifactS3Endpoint)
		bad(endpointErr == nil && endpoint.Host != "" && endpoint.User == nil && endpoint.Path == "" && endpoint.RawQuery == "" && endpoint.Fragment == "", "ARTIFACT_S3_ENDPOINT must be an absolute URL without path, userinfo, query, or fragment")
		bad(c.ArtifactS3Bucket != "" && strings.TrimSpace(c.ArtifactS3Bucket) == c.ArtifactS3Bucket && !strings.ContainsAny(c.ArtifactS3Bucket, "/\\:%"), "ARTIFACT_S3_BUCKET is invalid")
		// staging-sim has role-specific validation: credentials belong only in
		// the Worker process, never in the API process environment.
		if c.Profile != "staging-sim" {
			bad(c.ArtifactS3AccessKey != "" && c.ArtifactS3SecretKey != "", "ARTIFACT_S3_ACCESS_KEY and ARTIFACT_S3_SECRET_KEY are required for S3")
		}
	}
	if c.ArtifactBackend == "oss" {
		endpoint, endpointErr := url.Parse(c.ArtifactOSSEndpoint)
		bad(endpointErr == nil && endpoint.Scheme == "https" && endpoint.Host != "" && endpoint.User == nil && endpoint.Path == "" && endpoint.RawQuery == "" && endpoint.Fragment == "", "ARTIFACT_OSS_ENDPOINT must be an HTTPS regional endpoint without path or credentials")
		bad(c.ArtifactOSSRegion != "" && strings.TrimSpace(c.ArtifactOSSRegion) == c.ArtifactOSSRegion, "ARTIFACT_OSS_REGION is required")
		bad(c.ArtifactOSSBucket != "" && strings.TrimSpace(c.ArtifactOSSBucket) == c.ArtifactOSSBucket && !strings.ContainsAny(c.ArtifactOSSBucket, "/\\:%"), "ARTIFACT_OSS_BUCKET is invalid")
		if c.ArtifactOSSPrefix != "" {
			bad(!strings.Contains(c.ArtifactOSSPrefix, "..") && !strings.HasPrefix(c.ArtifactOSSPrefix, "/") && !strings.ContainsAny(c.ArtifactOSSPrefix, "\\%"), "ARTIFACT_OSS_PREFIX is invalid")
		}
	}
	if c.EvalBackend == "gateway" {
		gatewayURL, gatewayErr := url.Parse(c.EvalGatewayURL)
		bad(gatewayErr == nil && gatewayURL.Host != "" && (gatewayURL.Scheme == "http" || gatewayURL.Scheme == "https"), "EVAL_GATEWAY_URL must be an absolute HTTP(S) URL for gateway backend")
		bad(strings.TrimSpace(c.EvalGatewayProfile) != "", "EVAL_GATEWAY_PROFILE is required for gateway backend")
	}

	u, urlErr := url.Parse(c.DatabaseURL)
	bad(urlErr == nil && u.Scheme == "postgres" || urlErr == nil && u.Scheme == "postgresql", "DATABASE_URL must be a PostgreSQL URL")
	if c.Profile == "production" {
		bad(!c.AutoMigrate, "AUTO_MIGRATE must be false in production; use the separate migration job")
		bad(len(c.CursorSigningKey) >= 32 || c.CursorRotationEnabled, "API_CURSOR_SIGNING_KEY or an enabled cursor key ring is required in production")
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
		bad(c.ArtifactBackend == "s3" || c.ArtifactBackend == "oss", "ARTIFACT_STORE_BACKEND must be s3 or oss in production")
		if c.ArtifactBackend == "s3" {
			artifactEndpoint, artifactErr := url.Parse(c.ArtifactS3Endpoint)
			bad(artifactErr == nil && artifactEndpoint.Scheme == "https" && artifactEndpoint.Host != "" && artifactEndpoint.User == nil && artifactEndpoint.Path == "" && artifactEndpoint.RawQuery == "" && artifactEndpoint.Fragment == "", "ARTIFACT_S3_ENDPOINT must use HTTPS and contain no path or credentials in production")
			bad(c.ArtifactS3Bucket != "" && c.ArtifactS3AccessKey != "" && c.ArtifactS3SecretKey != "", "S3 bucket and credentials are required in production")
		}
		bad(!c.ArtifactAllowHTTP && !c.ArtifactAllowBucketCreate, "HTTP endpoints and automatic bucket creation are forbidden in production")
		if c.ArtifactBackend == "oss" {
			bad(c.ArtifactOSSRAMRoleName != "" && c.ArtifactS3AccessKey == "" && c.ArtifactS3SecretKey == "", "production OSS access requires an ECS RAM role and forbids static S3 credentials")
		}
		bad(c.EvalBackend == "gateway", "EVAL_BACKEND must be gateway in production")
		gatewayURL, gatewayErr := url.Parse(c.EvalGatewayURL)
		bad(gatewayErr == nil && gatewayURL.Scheme == "https" && gatewayURL.Host != "", "EVAL_GATEWAY_URL must use HTTPS in production")
		bad(strings.TrimSpace(c.EvalGatewayAPIKey) != "", "EVAL_GATEWAY_API_KEY is required in production")
	}
	if c.Profile == "staging-sim" {
		// The local profile never accepts static API credentials; its API-side
		// identity requirements are checked by ValidateAPI.
		bad(c.APIToken == "" && c.AdminToken == "", "static API_TOKEN and ADMIN_TOKEN are forbidden in staging-sim")
		bad(!c.AutoMigrate, "AUTO_MIGRATE must be false in staging-sim; use the separate migration job")
		if c.StagingSimCloud {
			databaseURL, databaseErr := url.Parse(c.DatabaseURL)
			bad(databaseErr == nil && isPrivateNetworkHost(databaseURL.Hostname()) && databaseURL.Port() != "" && (databaseURL.Query().Get("sslmode") == "verify-ca" || databaseURL.Query().Get("sslmode") == "verify-full") && strings.TrimSpace(databaseURL.Query().Get("sslrootcert")) != "", "cloud staging-sim DATABASE_URL must use a private IP endpoint, sslmode=verify-ca or verify-full, and an explicit sslrootcert")
		}
		if c.ArtifactBackend == "oss" {
			bad(c.ArtifactOSSRAMRoleName != "" && c.ArtifactS3AccessKey == "" && c.ArtifactS3SecretKey == "", "staging-sim OSS access requires an ECS RAM role and forbids static S3 credentials")
			bad(!c.ArtifactAllowHTTP && !c.ArtifactAllowBucketCreate, "staging-sim OSS requires HTTPS and a pre-provisioned bucket")
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n - %s", strings.Join(errs, "\n - "))
}

// ValidateAPI adds API-only identity, cursor, and transport requirements.
// Keeping these separate allows the staging simulation Worker to receive no
// OIDC private/client material or API TLS private key.
func (c *Config) ValidateAPI() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Profile != "staging-sim" {
		return nil
	}
	errs := make([]string, 0, 7)
	issuer, issuerErr := url.Parse(c.OIDCIssuer)
	if issuerErr != nil || issuer.Scheme != "https" || issuer.Hostname() != "identity" || issuer.Port() != "8443" {
		errs = append(errs, "staging-sim OIDC_ISSUER must use the local HTTPS identity service on port 8443")
	}
	jwks, jwksErr := url.Parse(c.OIDCJWKSURL)
	if jwksErr != nil || jwks.Scheme != "https" || jwks.Hostname() != "identity" || jwks.Port() != "8443" {
		errs = append(errs, "staging-sim OIDC_JWKS_URL must use the local HTTPS identity service on port 8443")
	}
	if strings.TrimSpace(c.OIDCAudience) == "" {
		errs = append(errs, "OIDC_AUDIENCE is required in staging-sim")
	}
	if strings.TrimSpace(c.OIDCCAFile) == "" {
		errs = append(errs, "OIDC_CA_CERT_FILE is required in staging-sim")
	}
	if c.OIDCRoleClaim != "roles" && c.OIDCRoleClaim != "groups" {
		errs = append(errs, "OIDC_ROLE_CLAIM must be roles or groups")
	}
	if len(c.OIDCRoleMappings) == 0 {
		errs = append(errs, "OIDC_ROLE_MAPPINGS must explicitly map trusted claims in staging-sim")
	}
	if len(c.CursorSigningKey) < 32 && !c.CursorRotationEnabled {
		errs = append(errs, "API_CURSOR_SIGNING_KEY or an enabled cursor key ring is required in staging-sim")
	}
	if strings.TrimSpace(c.APITLSCertFile) == "" || strings.TrimSpace(c.APITLSKeyFile) == "" {
		errs = append(errs, "API_TLS_CERT_FILE and API_TLS_KEY_FILE are required in staging-sim")
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid API configuration:\n - %s", strings.Join(errs, "\n - "))
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
	if c.Profile == "staging-sim" {
		errs := make([]string, 0, 12)
		u, _ := url.Parse(c.DatabaseURL)
		if !c.StagingSimCloud && (u == nil || u.Hostname() != "postgres" || u.Port() != "5432" || u.Query().Get("sslmode") != "disable") {
			errs = append(errs, "local staging-sim DATABASE_URL must target the isolated postgres service on port 5432 with explicit sslmode=disable")
		}
		if c.ArtifactBackend == "s3" {
			artifactEndpoint, artifactErr := url.Parse(c.ArtifactS3Endpoint)
			if artifactErr != nil || artifactEndpoint.Scheme != "http" || artifactEndpoint.Hostname() != "minio" || artifactEndpoint.Port() != "9000" || artifactEndpoint.User != nil || artifactEndpoint.Path != "" || artifactEndpoint.RawQuery != "" || artifactEndpoint.Fragment != "" {
				errs = append(errs, "staging-sim S3 must target the isolated MinIO service over HTTP on port 9000")
			}
			if c.ArtifactS3Bucket == "" || c.ArtifactS3AccessKey == "" || c.ArtifactS3SecretKey == "" || !c.ArtifactAllowHTTP || c.ArtifactAllowBucketCreate {
				errs = append(errs, "staging-sim MinIO requires S3 credentials, ARTIFACT_ALLOW_HTTP=true, and ARTIFACT_ALLOW_BUCKET_CREATE=false")
			}
		} else if c.ArtifactBackend == "oss" {
			artifactEndpoint, artifactErr := url.Parse(c.ArtifactOSSEndpoint)
			if artifactErr != nil || artifactEndpoint.Scheme != "https" || artifactEndpoint.Host == "" || c.ArtifactOSSRegion == "" || c.ArtifactOSSBucket == "" || c.ArtifactOSSRAMRoleName == "" || c.ArtifactAllowHTTP || c.ArtifactAllowBucketCreate {
				errs = append(errs, "staging-sim OSS requires a regional HTTPS endpoint, region, pre-provisioned bucket, ECS RAM role, and disabled HTTP/bucket creation")
			}
		} else {
			errs = append(errs, "staging-sim requires the MinIO S3 backend or the explicit OSS backend")
		}
		gatewayURL, gatewayErr := url.Parse(c.EvalGatewayURL)
		if c.StagingSimCloud {
			if c.EvalBackend != "gateway" || gatewayErr != nil || (gatewayURL.Scheme != "http" && gatewayURL.Scheme != "https") || !isPrivateNetworkHost(gatewayURL.Hostname()) || gatewayURL.Port() == "" || gatewayURL.User != nil || gatewayURL.Path != "" || gatewayURL.RawQuery != "" || gatewayURL.Fragment != "" {
				errs = append(errs, "cloud staging-sim EVAL_BACKEND and EVAL_GATEWAY_URL must target an explicit private gateway endpoint")
			}
		} else if c.EvalBackend != "gateway" || gatewayErr != nil || gatewayURL.Scheme != "http" || gatewayURL.Hostname() != "gateway-simulator" || gatewayURL.Port() != "8081" || gatewayURL.User != nil || gatewayURL.Path != "" || gatewayURL.RawQuery != "" || gatewayURL.Fragment != "" {
			errs = append(errs, "local staging-sim EVAL_BACKEND and EVAL_GATEWAY_URL must target the isolated gateway-simulator service over HTTP on port 8081")
		}
		if c.EvalGatewayProfile != "staging-sim" || strings.TrimSpace(c.EvalGatewayAPIKey) == "" {
			errs = append(errs, "staging-sim EVAL_GATEWAY_PROFILE and EVAL_GATEWAY_API_KEY are required")
		}
		if len(errs) > 0 {
			return fmt.Errorf("invalid worker configuration:\n - %s", strings.Join(errs, "\n - "))
		}
		return nil
	}
	return nil
}

func isLocalDatabaseHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "postgres"
}

func isPrivateNetworkHost(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsPrivate()
}

func isLocalArtifactDir(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "" || !(strings.HasPrefix(v, "s3://") || strings.HasPrefix(v, "minio://"))
}

// EffectiveChecksum returns a stable fingerprint of non-secret settings for
// correlation with logs and deployments. Tokens and database credentials are
// intentionally excluded.
func (c *Config) EffectiveChecksum() string {
	previousIDs := make([]string, 0, len(c.CursorPreviousKeys))
	for _, previous := range c.CursorPreviousKeys {
		previousIDs = append(previousIDs, previous.ID+"@"+previous.AcceptUntil.Format(time.RFC3339))
	}
	value := strings.Join([]string{c.Profile, strconv.FormatBool(c.StagingSimCloud), c.DatabaseURLWithoutCredentials(), strconv.Itoa(c.APIPort), strconv.Itoa(c.Concurrency), c.PollInterval.String(), c.LeaseTimeout.String(), c.ArtifactBackend, c.ArtifactS3Endpoint, c.ArtifactS3Bucket, c.ArtifactS3Prefix, c.ArtifactOSSEndpoint, c.ArtifactOSSRegion, c.ArtifactOSSBucket, c.ArtifactOSSPrefix, c.ArtifactOSSRAMRoleName, strconv.FormatInt(c.ArtifactMaxObjectBytes, 10), c.ArtifactDir, c.EvalBackend, c.EvalGatewayURL, c.EvalGatewayProfile, strconv.FormatBool(c.SharedRateLimitEnabled), strconv.FormatFloat(c.SharedRateLimitRate, 'f', -1, 64), strconv.Itoa(c.SharedRateLimitBurst), c.SharedRateLimitFailureMode, strconv.FormatBool(c.CursorRotationEnabled), c.CursorActiveKeyID, c.CursorTTL.String(), strings.Join(previousIDs, ","), c.CursorLegacyV1AcceptUntil.Format(time.RFC3339)}, "|")
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

// SensitiveValues returns configured credentials for in-process log redaction.
// Callers must not persist or print the returned values.
func (c *Config) SensitiveValues() []string {
	values := []string{c.DatabaseURL, c.APIToken, c.AdminToken, c.CursorSigningKey,
		c.ArtifactS3AccessKey, c.ArtifactS3SecretKey, c.EvalGatewayAPIKey, c.SharedRateLimitHashKey,
		c.CursorActiveKey, c.CursorLegacyV1Key, c.CursorPreviousKeysRaw}
	for _, previous := range c.CursorPreviousKeys {
		values = append(values, previous.Key)
	}
	if parsed, err := url.Parse(c.DatabaseURL); err == nil && parsed.User != nil {
		if password, ok := parsed.User.Password(); ok {
			values = append(values, password)
		}
	}
	filtered := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
