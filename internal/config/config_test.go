package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultWorkerIDIsUniquePerLoad(t *testing.T) {
	t.Setenv("WORKER_ID", "")
	a, b := Load().WorkerID, Load().WorkerID
	if a == b {
		t.Fatalf("default worker IDs collided: %q", a)
	}
	if !strings.Contains(a, "-") {
		t.Fatalf("worker ID lacks instance identity: %q", a)
	}
}

func TestSensitiveValuesIncludesConfiguredCredentialsForLogRedaction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:database-secret@db.example:5432/tasks?sslmode=require")
	t.Setenv("API_TOKEN", "api-token-secret")
	t.Setenv("ADMIN_TOKEN", "admin-token-secret")
	t.Setenv("API_CURSOR_SIGNING_KEY", "cursor-signing-secret")
	t.Setenv("API_SHARED_RATE_LIMIT_HASH_KEY", "shared-rate-limit-hash-secret")
	t.Setenv("ARTIFACT_S3_ACCESS_KEY", "object-access-secret")
	t.Setenv("ARTIFACT_S3_SECRET_KEY", "object-secret-value")
	t.Setenv("EVAL_GATEWAY_API_KEY", "provider-secret-value")
	values := Load().SensitiveValues()
	for _, expected := range []string{"database-secret", "api-token-secret", "admin-token-secret", "cursor-signing-secret", "shared-rate-limit-hash-secret", "object-access-secret", "object-secret-value", "provider-secret-value"} {
		if !containsString(values, expected) {
			t.Fatalf("configured credential of length %d was absent from the redaction set", len(expected))
		}
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestValidateCollectsParseAndSafetyErrors(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("API_TOKEN", "")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_AUDIENCE", "")
	t.Setenv("OIDC_JWKS_URL", "")
	t.Setenv("OIDC_ROLE_MAPPINGS", "")
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable")
	t.Setenv("WORKER_CONCURRENCY", "many")
	t.Setenv("LEASE_TIMEOUT", "0s")
	t.Setenv("DB_OPERATION_TIMEOUT", "4h")
	t.Setenv("RAG_ARTIFACT_DIR", "artifacts/index")
	t.Setenv("ARTIFACT_STORE_BACKEND", "local")
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() unexpectedly succeeded")
	}
	for _, want := range []string{
		"WORKER_CONCURRENCY must be an integer", "LEASE_TIMEOUT must be greater than zero",
		"DB_OPERATION_TIMEOUT must be less than LEASE_TIMEOUT", "OIDC_ISSUER must be an absolute HTTPS URL",
		"sslmode=disable", "ARTIFACT_STORE_BACKEND must be s3",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error missing %q:\n%s", want, err)
		}
	}
}

func TestValidateProductionAcceptsStrictConfiguration(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("DATABASE_URL", "postgres://app:an-adequately-long-password@db.example:5432/tasklab?sslmode=require")
	t.Setenv("API_TOKEN", "api-token")
	t.Setenv("ADMIN_TOKEN", "admin-token")
	t.Setenv("API_TOKEN", "")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_AUDIENCE", "task-api")
	t.Setenv("OIDC_JWKS_URL", "https://issuer.example/jwks")
	t.Setenv("OIDC_ROLE_MAPPINGS", "task-reader:reader,task-ops:operator,task-admin:admin")
	t.Setenv("API_CURSOR_SIGNING_KEY", "phase1-test-only-key-with-at-least-32-bytes")
	t.Setenv("RAG_ARTIFACT_DIR", "s3://tasklab-artifacts/index")
	t.Setenv("ARTIFACT_STORE_BACKEND", "s3")
	t.Setenv("ARTIFACT_S3_ENDPOINT", "https://objects.example")
	t.Setenv("ARTIFACT_S3_BUCKET", "tasklab-artifacts")
	t.Setenv("ARTIFACT_S3_PREFIX", "index")
	t.Setenv("ARTIFACT_S3_ACCESS_KEY", "test-access")
	t.Setenv("ARTIFACT_S3_SECRET_KEY", "test-secret")
	t.Setenv("EVAL_BACKEND", "gateway")
	t.Setenv("EVAL_GATEWAY_URL", "https://eval.example")
	t.Setenv("EVAL_GATEWAY_PROFILE", "production-eval")
	t.Setenv("EVAL_GATEWAY_API_KEY", "secret-key")
	t.Setenv("API_SHARED_RATE_LIMIT_ENABLED", "false")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if cfg.EffectiveChecksum() == "" || strings.Contains(cfg.DatabaseURLWithoutCredentials(), "an-adequately") {
		t.Fatal("effective configuration leaked credentials or lacked checksum")
	}
	if err := cfg.ValidateWorker(); err == nil || !strings.Contains(err.Error(), "production execution allowlist is empty") {
		t.Fatalf("production worker must fail closed until a task type passes acceptance, got %v", err)
	}
}

func TestSharedRateLimitCandidateRequiresExplicitPolicyAndSecrets(t *testing.T) {
	t.Setenv("API_SHARED_RATE_LIMIT_ENABLED", "")
	t.Setenv("API_SHARED_RATE_LIMIT_RATE_PER_SECOND", "")
	t.Setenv("API_SHARED_RATE_LIMIT_BURST", "")
	t.Setenv("API_SHARED_RATE_LIMIT_FAILURE_MODE", "")
	t.Setenv("API_SHARED_RATE_LIMIT_HASH_KEY", "")
	if cfg := Load(); cfg.SharedRateLimitEnabled {
		t.Fatal("shared rate limit candidate must be disabled by default")
	}

	t.Setenv("API_SHARED_RATE_LIMIT_ENABLED", "true")
	t.Setenv("API_SHARED_RATE_LIMIT_RATE_PER_SECOND", "2")
	t.Setenv("API_SHARED_RATE_LIMIT_BURST", "20")
	cfg := Load()
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "FAILURE_MODE") || !strings.Contains(err.Error(), "HASH_KEY") {
		t.Fatalf("enabled candidate without explicit failure/key policy should fail closed, got %v", err)
	}

	t.Setenv("API_SHARED_RATE_LIMIT_FAILURE_MODE", "closed")
	t.Setenv("API_SHARED_RATE_LIMIT_HASH_KEY", strings.Repeat("k", 32))
	if err := Load().Validate(); err != nil {
		t.Fatalf("fully specified disabled-by-default candidate config should validate: %v", err)
	}
}

func TestCursorKeyRingIsOptInAndRequiresFiniteCompatibilityBoundaries(t *testing.T) {
	for _, name := range []string{"API_CURSOR_KEY_RING_ENABLED", "API_CURSOR_ACTIVE_KEY_ID", "API_CURSOR_ACTIVE_KEY", "API_CURSOR_TTL", "API_CURSOR_PREVIOUS_KEYS", "API_CURSOR_V1_LEGACY_KEY", "API_CURSOR_V1_ACCEPT_UNTIL"} {
		t.Setenv(name, "")
	}
	if cfg := Load(); cfg.CursorRotationEnabled {
		t.Fatal("cursor key ring must be disabled by default")
	}
	t.Setenv("API_CURSOR_KEY_RING_ENABLED", "true")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "ACTIVE_KEY_ID") || !strings.Contains(err.Error(), "API_CURSOR_TTL") {
		t.Fatalf("incomplete key ring config should fail closed, got %v", err)
	}
	t.Setenv("API_CURSOR_ACTIVE_KEY_ID", "active-2026-10")
	t.Setenv("API_CURSOR_ACTIVE_KEY", strings.Repeat("a", 32))
	t.Setenv("API_CURSOR_TTL", "1h")
	t.Setenv("API_CURSOR_PREVIOUS_KEYS", `[{"id":"previous-2026-09","key":"`+strings.Repeat("p", 32)+`","accept_until":"`+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+`"}]`)
	t.Setenv("API_CURSOR_V1_LEGACY_KEY", strings.Repeat("v", 32))
	t.Setenv("API_CURSOR_V1_ACCEPT_UNTIL", time.Now().UTC().Add(time.Minute).Format(time.RFC3339))
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("explicit cursor key ring config should validate: %v", err)
	}
	for _, secret := range []string{strings.Repeat("a", 32), strings.Repeat("p", 32), strings.Repeat("v", 32)} {
		if !containsString(cfg.SensitiveValues(), secret) {
			t.Fatal("cursor key was not added to the sensitive-value redaction list")
		}
	}
}

func TestValidateProductionRejectsLocalDatabaseEvenWithTLS(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("DATABASE_URL", "postgres://app:an-adequately-long-password@localhost:5432/tasklab?sslmode=require")
	t.Setenv("API_TOKEN", "api-token")
	t.Setenv("ADMIN_TOKEN", "admin-token")
	t.Setenv("API_TOKEN", "")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_AUDIENCE", "task-api")
	t.Setenv("OIDC_JWKS_URL", "https://issuer.example/jwks")
	t.Setenv("OIDC_ROLE_MAPPINGS", "task-admin:admin")
	t.Setenv("RAG_ARTIFACT_DIR", "s3://tasklab-artifacts/index")
	t.Setenv("ARTIFACT_STORE_BACKEND", "s3")
	t.Setenv("ARTIFACT_S3_ENDPOINT", "https://objects.example")
	t.Setenv("ARTIFACT_S3_BUCKET", "tasklab-artifacts")
	t.Setenv("ARTIFACT_S3_ACCESS_KEY", "test-access")
	t.Setenv("ARTIFACT_S3_SECRET_KEY", "test-secret")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "local database host") {
		t.Fatalf("expected local host rejection, got %v", err)
	}
}

func TestValidateProductionRejectsDeterministicEvalFallback(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("DATABASE_URL", "postgres://app:an-adequately-long-password@db.example:5432/tasklab?sslmode=require")
	t.Setenv("API_TOKEN", "api-token")
	t.Setenv("ADMIN_TOKEN", "admin-token")
	t.Setenv("API_TOKEN", "")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_AUDIENCE", "task-api")
	t.Setenv("OIDC_JWKS_URL", "https://issuer.example/jwks")
	t.Setenv("OIDC_ROLE_MAPPINGS", "task-admin:admin")
	t.Setenv("RAG_ARTIFACT_DIR", "s3://tasklab-artifacts/index")
	t.Setenv("ARTIFACT_STORE_BACKEND", "s3")
	t.Setenv("ARTIFACT_S3_ENDPOINT", "https://objects.example")
	t.Setenv("ARTIFACT_S3_BUCKET", "tasklab-artifacts")
	t.Setenv("ARTIFACT_S3_ACCESS_KEY", "test-access")
	t.Setenv("ARTIFACT_S3_SECRET_KEY", "test-secret")
	t.Setenv("EVAL_BACKEND", "deterministic")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "EVAL_BACKEND must be gateway") {
		t.Fatalf("expected deterministic fallback rejection, got %v", err)
	}
}

func TestProductionRejectsUnsafeArtifactEndpointAndStaticAuth(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("DATABASE_URL", "postgres://app:an-adequately-long-password@db.example:5432/tasklab?sslmode=require")
	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_AUDIENCE", "task-api")
	t.Setenv("OIDC_JWKS_URL", "https://issuer.example/jwks")
	t.Setenv("OIDC_ROLE_MAPPINGS", "task-admin:admin")
	t.Setenv("ARTIFACT_STORE_BACKEND", "s3")
	t.Setenv("ARTIFACT_S3_ENDPOINT", "http://objects.example")
	t.Setenv("ARTIFACT_S3_BUCKET", "tasklab")
	t.Setenv("ARTIFACT_S3_ACCESS_KEY", "access")
	t.Setenv("ARTIFACT_S3_SECRET_KEY", "secret")
	t.Setenv("API_TOKEN", "legacy-token")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "static authentication") || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected static-auth and HTTPS failures, got %v", err)
	}
}

func TestValidateCrossFieldLimits(t *testing.T) {
	cfg := Load()
	cfg.Concurrency = 3
	cfg.DBMaxOpenConns = 2
	cfg.DBMaxIdleConns = 3
	cfg.DBOperationTimeout = 5 * time.Second
	cfg.LeaseTimeout = 5 * time.Second
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "WORKER_CONCURRENCY must not exceed") || !strings.Contains(err.Error(), "DB_MAX_IDLE_CONNS") || !strings.Contains(err.Error(), "DB_OPERATION_TIMEOUT") {
		t.Fatalf("expected all cross-field errors, got %v", err)
	}
}

func TestExplicitWorkerIDOverride(t *testing.T) {
	t.Setenv("WORKER_ID", "stable-instance-id")
	if got := Load().WorkerID; got != "stable-instance-id" {
		t.Fatalf("WorkerID=%q", got)
	}
}
