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

func TestValidateCollectsParseAndSafetyErrors(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("API_TOKEN", "")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable")
	t.Setenv("WORKER_CONCURRENCY", "many")
	t.Setenv("LEASE_TIMEOUT", "0s")
	t.Setenv("DB_OPERATION_TIMEOUT", "4h")
	t.Setenv("RAG_ARTIFACT_DIR", "artifacts/index")
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() unexpectedly succeeded")
	}
	for _, want := range []string{
		"WORKER_CONCURRENCY must be an integer", "LEASE_TIMEOUT must be greater than zero",
		"DB_OPERATION_TIMEOUT must be less than LEASE_TIMEOUT", "API_TOKEN is required in production",
		"sslmode=disable", "shared object storage",
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
	t.Setenv("RAG_ARTIFACT_DIR", "s3://tasklab-artifacts/index")
	t.Setenv("EVAL_BACKEND", "gateway")
	t.Setenv("EVAL_GATEWAY_URL", "https://eval.example")
	t.Setenv("EVAL_GATEWAY_PROFILE", "production-eval")
	t.Setenv("EVAL_GATEWAY_API_KEY", "secret-key")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if cfg.EffectiveChecksum() == "" || strings.Contains(cfg.DatabaseURLWithoutCredentials(), "an-adequately") {
		t.Fatal("effective configuration leaked credentials or lacked checksum")
	}
	if err := cfg.ValidateWorker(); err == nil || !strings.Contains(err.Error(), "shared artifact adapter is not implemented") {
		t.Fatalf("production worker must fail closed until shared artifacts are implemented, got %v", err)
	}
}

func TestValidateProductionRejectsLocalDatabaseEvenWithTLS(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("DATABASE_URL", "postgres://app:an-adequately-long-password@localhost:5432/tasklab?sslmode=require")
	t.Setenv("API_TOKEN", "api-token")
	t.Setenv("ADMIN_TOKEN", "admin-token")
	t.Setenv("RAG_ARTIFACT_DIR", "s3://tasklab-artifacts/index")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "local database host") {
		t.Fatalf("expected local host rejection, got %v", err)
	}
}

func TestValidateProductionRejectsDeterministicEvalFallback(t *testing.T) {
	t.Setenv("APP_PROFILE", "production")
	t.Setenv("DATABASE_URL", "postgres://app:an-adequately-long-password@db.example:5432/tasklab?sslmode=require")
	t.Setenv("API_TOKEN", "api-token")
	t.Setenv("ADMIN_TOKEN", "admin-token")
	t.Setenv("RAG_ARTIFACT_DIR", "s3://tasklab-artifacts/index")
	t.Setenv("EVAL_BACKEND", "deterministic")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "EVAL_BACKEND must be gateway") {
		t.Fatalf("expected deterministic fallback rejection, got %v", err)
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
