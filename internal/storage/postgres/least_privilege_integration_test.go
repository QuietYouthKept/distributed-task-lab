package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
)

func TestRuntimeRoleLeastPrivilege(t *testing.T) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate ephemeral role identity: %v", err)
	}
	suffix := hex.EncodeToString(random[:])
	roleName := "tasklab_runtime_" + suffix
	password := suffix + hex.EncodeToString(random[:])

	baseURL := testutil.DefaultBaseURL()
	database, err := testutil.EnsureDatabase(baseURL, "tasklab_runtime_role")
	if err != nil {
		if os.Getenv("REQUIRE_TEST_DATABASE") == "1" {
			t.Fatalf("required runtime role PostgreSQL integration unavailable: %v", err)
		}
		t.Skipf("skipping runtime role integration: cannot create PostgreSQL database: %v", err)
	}

	adminDatabase, err := NewDB(database.URL)
	if err != nil {
		database.Drop()
		t.Fatalf("connect as migration owner: %v", err)
	}
	if err := RunMigrations(adminDatabase); err != nil {
		adminDatabase.Close()
		database.Drop()
		t.Fatalf("migrate isolated database: %v", err)
	}

	baseAdmin, err := NewDB(baseURL)
	if err != nil {
		adminDatabase.Close()
		database.Drop()
		t.Fatalf("connect to maintenance database: %v", err)
	}
	roleCreated := false
	var runtimeDatabase *sql.DB
	t.Cleanup(func() {
		if runtimeDatabase != nil {
			_ = runtimeDatabase.Close()
		}
		_ = adminDatabase.Close()
		database.Drop()
		if roleCreated {
			_, _ = baseAdmin.Exec(`DROP ROLE IF EXISTS ` + quoteIdentifier(roleName))
		}
		_ = baseAdmin.Close()
	})

	// Both identifiers are generated from lowercase hex; the password never
	// enters logs or files. Role scope is limited to this disposable database.
	if _, err := baseAdmin.Exec(`CREATE ROLE ` + quoteIdentifier(roleName) + ` LOGIN PASSWORD '` + password + `'`); err != nil {
		t.Fatalf("create disposable runtime role: %v", err)
	}
	roleCreated = true
	grants := []string{
		`GRANT CONNECT ON DATABASE ` + quoteIdentifier(database.Name) + ` TO ` + quoteIdentifier(roleName),
		`GRANT USAGE ON SCHEMA public TO ` + quoteIdentifier(roleName),
		`GRANT SELECT, INSERT, UPDATE ON TABLE tasks, queues, workers, effects TO ` + quoteIdentifier(roleName),
		`GRANT SELECT, INSERT ON TABLE task_events, security_audit_events TO ` + quoteIdentifier(roleName),
		`GRANT USAGE ON SEQUENCE task_events_id_seq, security_audit_events_id_seq TO ` + quoteIdentifier(roleName),
	}
	for _, grant := range grants {
		if _, err := adminDatabase.Exec(grant); err != nil {
			t.Fatalf("apply runtime grant: %v", err)
		}
	}

	runtimeURL, err := url.Parse(database.URL)
	if err != nil {
		t.Fatalf("parse isolated database URL: %v", err)
	}
	runtimeURL.User = url.UserPassword(roleName, password)
	runtimeDatabase, err = NewDB(runtimeURL.String())
	if err != nil {
		t.Fatalf("connect using restricted runtime role: %v", err)
	}
	runtimeQueue := NewPostgresQueue(runtimeDatabase)
	ctx := context.Background()
	task, err := runtimeQueue.Enqueue(ctx, newTask("runtime-role-lifecycle", 2))
	if err != nil {
		t.Fatalf("runtime enqueue: %v", err)
	}
	claimed := claimOne(t, runtimeQueue, "runtime-role-worker")
	if claimed.ID != task.ID {
		t.Fatalf("runtime role claimed %q, want %q", claimed.ID, task.ID)
	}
	if err := runtimeQueue.MarkSuccess(ctx, task.ID, "runtime-role-worker", claimed.LeaseVersion, domain.TaskResult{Summary: "least privilege verified"}); err != nil {
		t.Fatalf("runtime ACK: %v", err)
	}

	operatorCtx := auth.WithPrincipal(contract.WithRequestID(ctx, "runtime-role-request-0001"), auth.Principal{
		Subject: "runtime-role-operator",
		Kind:    auth.KindService,
		Roles:   map[auth.Role]struct{}{auth.RoleOperator: {}},
	})
	if err := runtimeQueue.PauseQueue(operatorCtx, domain.DefaultQueueName); err != nil {
		t.Fatalf("runtime administrative audit insert: %v", err)
	}
	if err := runtimeQueue.ResumeQueue(operatorCtx, domain.DefaultQueueName); err != nil {
		t.Fatalf("runtime audit insert on resume: %v", err)
	}
	var auditCount int
	if err := runtimeDatabase.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_audit_events WHERE resource_id=$1`, domain.DefaultQueueName).Scan(&auditCount); err != nil {
		t.Fatalf("runtime audit read: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("runtime audit rows=%d, want 2", auditCount)
	}
	var isSuperuser, canCreateRole, canCreateDB bool
	if err := runtimeDatabase.QueryRowContext(ctx, `SELECT rolsuper, rolcreaterole, rolcreatedb FROM pg_roles WHERE rolname=current_user`).Scan(&isSuperuser, &canCreateRole, &canCreateDB); err != nil {
		t.Fatalf("inspect runtime role attributes: %v", err)
	}
	if isSuperuser || canCreateRole || canCreateDB {
		t.Fatalf("runtime role has elevated cluster attributes: superuser=%v createrole=%v createdb=%v", isSuperuser, canCreateRole, canCreateDB)
	}

	for name, statement := range map[string]string{
		"delete task":            `DELETE FROM tasks WHERE id = 'runtime-role-lifecycle'`,
		"truncate tasks":         `TRUNCATE tasks`,
		"alter schema":           `CREATE TABLE runtime_role_must_not_create (id integer)`,
		"rewrite task event":     `UPDATE task_events SET message = 'tampered' WHERE task_id = 'runtime-role-lifecycle'`,
		"rewrite security audit": `UPDATE security_audit_events SET reason = 'tampered' WHERE resource_id = 'default'`,
		"delete security audit":  `DELETE FROM security_audit_events WHERE resource_id = 'default'`,
		"create cluster role":    `CREATE ROLE runtime_role_must_not_create`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runtimeDatabase.ExecContext(ctx, statement); err == nil {
				t.Fatalf("restricted runtime role unexpectedly succeeded: %s", statement)
			}
		})
	}
}

func TestRuntimeGrantScriptRestrictsWorkerQueueColumns(t *testing.T) {
	grantPath, err := filepath.Abs(filepath.Join("..", "..", "..", "deployments", "local-first", "postgres", "runtime-grants.sql"))
	if err != nil {
		t.Fatalf("resolve runtime grant path: %v", err)
	}
	raw, err := os.ReadFile(grantPath)
	if err != nil {
		t.Fatalf("read runtime grant source %s: %v", grantPath, err)
	}
	grants := string(raw)
	if !strings.Contains(grants, "REVOKE UPDATE ON TABLE queues FROM tasklab_worker;") || !strings.Contains(grants, "GRANT UPDATE (rate_tokens, rate_updated_at) ON TABLE queues TO tasklab_worker;") {
		t.Fatal("runtime grant script must revoke broad Worker queue updates and grant only rate-token state columns")
	}
	if strings.Contains(grants, "GRANT SELECT, UPDATE ON TABLE queues TO tasklab_worker;") {
		t.Fatal("runtime grant script still contains a table-wide Worker UPDATE grant on queues")
	}
}

// TestWorkerCannotRewriteQueueGovernance verifies that the Worker role may
// persist the shared token bucket without being able to pause queues or alter
// operator-owned concurrency/rate/aging policy.
func TestWorkerCannotRewriteQueueGovernance(t *testing.T) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate ephemeral role identity: %v", err)
	}
	suffix := hex.EncodeToString(random[:])
	roleName := "tasklab_queue_worker_" + suffix
	password := suffix + hex.EncodeToString(random[:])
	database, err := testutil.EnsureDatabase(testutil.DefaultBaseURL(), "tasklab_queue_policy")
	if err != nil {
		if os.Getenv("REQUIRE_TEST_DATABASE") == "1" {
			t.Fatalf("required queue policy privilege integration unavailable: %v", err)
		}
		t.Skipf("skipping queue policy privilege integration: cannot create isolated PostgreSQL database: %v", err)
	}
	adminDatabase, err := NewDB(database.URL)
	if err != nil {
		database.Drop()
		t.Fatalf("connect to isolated migration database: %v", err)
	}
	if err := RunMigrations(adminDatabase); err != nil {
		adminDatabase.Close()
		database.Drop()
		t.Fatalf("migrate isolated database: %v", err)
	}
	baseAdmin, err := NewDB(testutil.DefaultBaseURL())
	if err != nil {
		adminDatabase.Close()
		database.Drop()
		t.Fatalf("connect to maintenance database: %v", err)
	}
	roleCreated := false
	var workerDatabase *sql.DB
	t.Cleanup(func() {
		if workerDatabase != nil {
			_ = workerDatabase.Close()
		}
		_ = adminDatabase.Close()
		database.Drop()
		if roleCreated {
			_, _ = baseAdmin.Exec(`DROP ROLE IF EXISTS ` + quoteIdentifier(roleName))
		}
		_ = baseAdmin.Close()
	})
	if _, err := baseAdmin.Exec(`CREATE ROLE ` + quoteIdentifier(roleName) + ` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD '` + password + `'`); err != nil {
		t.Fatalf("create disposable Worker role: %v", err)
	}
	roleCreated = true
	for _, statement := range []string{
		`GRANT CONNECT ON DATABASE ` + quoteIdentifier(database.Name) + ` TO ` + quoteIdentifier(roleName),
		`GRANT USAGE ON SCHEMA public TO ` + quoteIdentifier(roleName),
		`GRANT SELECT, UPDATE ON TABLE queues TO ` + quoteIdentifier(roleName),
		`REVOKE UPDATE ON TABLE queues FROM ` + quoteIdentifier(roleName),
		`GRANT SELECT ON TABLE queues TO ` + quoteIdentifier(roleName),
		`GRANT UPDATE (rate_tokens, rate_updated_at) ON TABLE queues TO ` + quoteIdentifier(roleName),
	} {
		if _, err := adminDatabase.Exec(statement); err != nil {
			t.Fatalf("apply Worker queue grant: %v", err)
		}
	}
	workerURL, err := url.Parse(database.URL)
	if err != nil {
		t.Fatalf("parse isolated database URL: %v", err)
	}
	workerURL.User = url.UserPassword(roleName, password)
	workerDatabase, err = NewDB(workerURL.String())
	if err != nil {
		t.Fatalf("connect using restricted Worker role: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for name, statement := range map[string]string{
		"change max concurrency": `UPDATE queues SET max_concurrency=max_concurrency+1 WHERE name='default'`,
		"pause queue":            `UPDATE queues SET paused=true WHERE name='default'`,
		"change rate policy":     `UPDATE queues SET rate_limit_per_second=1000 WHERE name='default'`,
		"change priority aging":  `UPDATE queues SET aging_priority_boost=1000 WHERE name='default'`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := workerDatabase.ExecContext(ctx, statement); err == nil {
				t.Fatalf("Worker role unexpectedly changed operator-owned queue policy: %s", statement)
			}
		})
	}
	if _, err := workerDatabase.ExecContext(ctx, `UPDATE queues SET rate_tokens=rate_tokens, rate_updated_at=clock_timestamp() WHERE name='default'`); err != nil {
		t.Fatalf("Worker role could not persist shared limiter state: %v", err)
	}
	var canUpdatePolicy, canUpdateRateTokens bool
	if err := workerDatabase.QueryRowContext(ctx, `SELECT has_column_privilege(current_user,'queues','max_concurrency','UPDATE'), has_column_privilege(current_user,'queues','rate_tokens','UPDATE')`).Scan(&canUpdatePolicy, &canUpdateRateTokens); err != nil {
		t.Fatalf("inspect Worker column grants: %v", err)
	}
	if canUpdatePolicy || !canUpdateRateTokens {
		t.Fatalf("unexpected Worker queue column grants: policy_update=%v rate_tokens_update=%v", canUpdatePolicy, canUpdateRateTokens)
	}
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
