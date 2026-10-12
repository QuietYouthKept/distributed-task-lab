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
	if !strings.Contains(grants, "REVOKE UPDATE ON TABLE tasks FROM tasklab_worker;") {
		t.Fatal("runtime grant script must revoke direct Worker task updates")
	}
	if !strings.Contains(grants, "GRANT EXECUTE ON FUNCTION tasklab_worker_ack(TEXT, INTEGER, JSONB) TO tasklab_worker;") {
		t.Fatal("runtime grant script must grant the fenced ACK routine")
	}
	if strings.Contains(grants, "GRANT SELECT, INSERT ON TABLE task_events TO tasklab_worker;") || !strings.Contains(grants, "GRANT EXECUTE ON FUNCTION tasklab_worker_insert_event(TEXT, TEXT, TEXT, JSONB) TO tasklab_worker;") {
		t.Fatal("runtime grant script must route Worker event writes through identity-bound routine")
	}
}

// TestFencedWorkerDatabaseTransitions proves the production Worker path cannot
// directly mutate task state and that every legal transition is bound to the
// authenticated PostgreSQL login rather than a caller-provided worker ID.
func TestFencedWorkerDatabaseTransitions(t *testing.T) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("generate disposable Worker identity: %v", err)
	}
	suffix := hex.EncodeToString(random[:])
	roleName := "tasklab_fenced_" + suffix
	workerID := "fenced-worker-" + suffix[:12]
	password := suffix + hex.EncodeToString(random[:])
	database, err := testutil.EnsureDatabase(testutil.DefaultBaseURL(), "tasklab_fenced_worker")
	if err != nil {
		if os.Getenv("REQUIRE_TEST_DATABASE") == "1" {
			t.Fatalf("required fenced Worker PostgreSQL integration unavailable: %v", err)
		}
		t.Skipf("skipping fenced Worker integration: cannot create isolated PostgreSQL database: %v", err)
	}
	adminDB, err := NewDB(database.URL)
	if err != nil {
		database.Drop()
		t.Fatalf("connect isolated database owner: %v", err)
	}
	if err := RunMigrations(adminDB); err != nil {
		adminDB.Close()
		database.Drop()
		t.Fatalf("apply migrations: %v", err)
	}
	baseAdmin, err := NewDB(testutil.DefaultBaseURL())
	if err != nil {
		adminDB.Close()
		database.Drop()
		t.Fatalf("connect maintenance database: %v", err)
	}
	var workerDB *sql.DB
	roleCreated := false
	t.Cleanup(func() {
		if workerDB != nil {
			_ = workerDB.Close()
		}
		_ = adminDB.Close()
		database.Drop()
		if roleCreated {
			_, _ = baseAdmin.Exec(`DROP ROLE IF EXISTS ` + quoteIdentifier(roleName))
		}
		_ = baseAdmin.Close()
	})
	if _, err := baseAdmin.Exec(`CREATE ROLE ` + quoteIdentifier(roleName) + ` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD '` + password + `'`); err != nil {
		t.Fatalf("create disposable Worker login: %v", err)
	}
	roleCreated = true
	for _, statement := range []string{
		`GRANT CONNECT ON DATABASE ` + quoteIdentifier(database.Name) + ` TO ` + quoteIdentifier(roleName),
		`GRANT USAGE ON SCHEMA public TO ` + quoteIdentifier(roleName),
		`GRANT SELECT ON TABLE tasks, queues, schema_migrations TO ` + quoteIdentifier(roleName),
		`GRANT INSERT ON TABLE workers TO ` + quoteIdentifier(roleName),
		`GRANT SELECT ON TABLE task_events TO ` + quoteIdentifier(roleName),
		`GRANT UPDATE ON TABLE workers TO ` + quoteIdentifier(roleName),
		`GRANT SELECT, INSERT, UPDATE ON TABLE effects TO ` + quoteIdentifier(roleName),
		`GRANT UPDATE (rate_tokens, rate_updated_at) ON TABLE queues TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_identity() TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_terminalize_exhausted_leases() TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_claim_task(TEXT, BIGINT) TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_ack(TEXT, INTEGER, JSONB) TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_fail(TEXT, INTEGER, TEXT, TEXT, BOOLEAN) TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_renew(TEXT, INTEGER, BIGINT) TO ` + quoteIdentifier(roleName),
		`GRANT EXECUTE ON FUNCTION tasklab_worker_insert_event(TEXT, TEXT, TEXT, JSONB) TO ` + quoteIdentifier(roleName),
	} {
		if _, err := adminDB.Exec(statement); err != nil {
			t.Fatalf("apply scoped Worker grant: %v", err)
		}
	}
	if _, err := adminDB.Exec(`INSERT INTO worker_db_identities(login_role, worker_id) VALUES ($1, $2)`, roleName, workerID); err != nil {
		t.Fatalf("bind database login to Worker identity: %v", err)
	}
	workerURL, err := url.Parse(database.URL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	workerURL.User = url.UserPassword(roleName, password)
	workerDB, err = NewDB(workerURL.String())
	if err != nil {
		t.Fatalf("connect as registered Worker: %v", err)
	}
	ctx := context.Background()
	workerQueue, err := NewFencedWorkerQueue(ctx, workerDB, 5*time.Second, workerID)
	if err != nil {
		t.Fatalf("initialize fenced Worker queue: %v", err)
	}
	adminQueue := NewPostgresQueue(adminDB)

	task := newTask("fenced-worker-ack", 3)
	if _, err := adminQueue.Enqueue(ctx, task); err != nil {
		t.Fatalf("enqueue ACK task: %v", err)
	}
	if _, err := workerDB.ExecContext(ctx, `UPDATE tasks SET status='success' WHERE id=$1`, task.ID); err == nil {
		t.Fatal("Worker login unexpectedly performed direct task state UPDATE")
	}
	if _, err := workerDB.ExecContext(ctx, `INSERT INTO task_events(task_id,event_type,worker_id,message) VALUES ($1,'succeeded','spoofed-worker','forged')`, task.ID); err == nil {
		t.Fatal("Worker login unexpectedly inserted an event with a caller-forged identity")
	}
	if _, err := workerDB.ExecContext(ctx, `SELECT tasklab_worker_insert_event($1,'claimed','forged','{}'::jsonb)`, task.ID); err == nil {
		t.Fatal("Worker event routine unexpectedly accepted an event inconsistent with task state")
	}
	claimed := claimOne(t, workerQueue, workerID)
	if claimed.Attempt != 1 || claimed.LeaseVersion != 1 || claimed.LeasedBy == nil || *claimed.LeasedBy != workerID {
		t.Fatalf("unexpected first claim identity/generation: attempt=%d lease_version=%d leased_by=%v", claimed.Attempt, claimed.LeaseVersion, claimed.LeasedBy)
	}
	if err := workerQueue.MarkSuccess(ctx, task.ID, "spoofed-worker", claimed.LeaseVersion, domain.TaskResult{Summary: "must reject spoof"}); err == nil {
		t.Fatal("queue accepted caller-supplied Worker identity that differs from the DB identity")
	}
	if err := workerQueue.RenewLease(ctx, task.ID, workerID, claimed.LeaseVersion, time.Second); err != nil {
		t.Fatalf("registered Worker lease renewal: %v", err)
	}
	if err := workerQueue.MarkSuccess(ctx, task.ID, workerID, claimed.LeaseVersion, domain.TaskResult{Summary: "fenced ACK"}); err != nil {
		t.Fatalf("registered Worker fenced ACK: %v", err)
	}
	ackEvents, err := adminQueue.GetTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("read ACK task events: %v", err)
	}
	var claimedEvents, successEvents int
	for _, event := range ackEvents {
		if event.EventType == domain.EventClaimed && event.WorkerID != nil && *event.WorkerID == workerID {
			claimedEvents++
		}
		if event.EventType == domain.EventSucceeded && event.WorkerID != nil && *event.WorkerID == workerID {
			successEvents++
		}
	}
	if claimedEvents != 1 || successEvents != 1 {
		t.Fatalf("transition trigger must atomically persist identity-bound events: claimed=%d succeeded=%d", claimedEvents, successEvents)
	}

	retryTask := newTask("fenced-worker-retry", 3)
	retryTask.RetryDelaySeconds = 30
	retryTask.RetryJitter = false
	if _, err := adminQueue.Enqueue(ctx, retryTask); err != nil {
		t.Fatalf("enqueue retry task: %v", err)
	}
	retryClaim := claimOne(t, workerQueue, workerID)
	if retryClaim.ID != retryTask.ID {
		t.Fatalf("claimed %q, want retry task %q", retryClaim.ID, retryTask.ID)
	}
	if err := workerQueue.MarkFailure(ctx, retryTask.ID, workerID, retryClaim.LeaseVersion, domain.TaskFailure{Message: "transient", ErrorType: "test", Retryable: true}); err != nil {
		t.Fatalf("registered Worker retry transition: %v", err)
	}
	retried, err := adminQueue.GetTask(ctx, retryTask.ID)
	if err != nil || retried == nil {
		t.Fatalf("read retried task: task=%v err=%v", retried, err)
	}
	var databaseNow time.Time
	if err := adminDB.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
		t.Fatalf("read database clock: %v", err)
	}
	if retried.Status != domain.TaskStatusPending || retried.Attempt != 1 || retried.LeaseVersion != retryClaim.LeaseVersion || retried.LeasedBy != nil || retried.LeaseExpiresAt != nil || !retried.NextRunAt.After(databaseNow) {
		t.Fatalf("unexpected retry invariants: status=%s attempt=%d lease_version=%d next_run_at=%s db_now=%s lease=%v expiry=%v", retried.Status, retried.Attempt, retried.LeaseVersion, retried.NextRunAt, databaseNow, retried.LeasedBy, retried.LeaseExpiresAt)
	}
	retryEvents, err := adminQueue.GetTaskEvents(ctx, retryTask.ID)
	if err != nil {
		t.Fatalf("read retry task events: %v", err)
	}
	var retryEventCount int
	for _, event := range retryEvents {
		if event.EventType == domain.EventRetried && event.WorkerID != nil && *event.WorkerID == workerID {
			retryEventCount++
		}
	}
	if retryEventCount != 1 {
		t.Fatalf("retry transition event count=%d, want 1 identity-bound event", retryEventCount)
	}

	staleTask := newTask("fenced-worker-stale-ack", 3)
	if _, err := adminQueue.Enqueue(ctx, staleTask); err != nil {
		t.Fatalf("enqueue stale ACK task: %v", err)
	}
	firstBatch, err := workerQueue.ClaimTasks(ctx, workerID, 1, 50*time.Millisecond)
	if err != nil || len(firstBatch) != 1 {
		t.Fatalf("claim stale-ACK task: tasks=%d err=%v", len(firstBatch), err)
	}
	first := firstBatch[0]
	if first.ID != staleTask.ID {
		t.Fatalf("claimed %q, want %q", first.ID, staleTask.ID)
	}
	time.Sleep(80 * time.Millisecond)
	secondBatch, err := workerQueue.ClaimTasks(ctx, workerID, 1, time.Second)
	if err != nil || len(secondBatch) != 1 {
		t.Fatalf("reclaim expired lease: tasks=%d err=%v", len(secondBatch), err)
	}
	second := secondBatch[0]
	if second.ID != staleTask.ID || second.LeaseVersion != first.LeaseVersion+1 {
		t.Fatalf("reclaim did not advance fencing generation: first=%+v second=%+v", first, second)
	}
	if err := workerQueue.MarkSuccess(ctx, staleTask.ID, workerID, first.LeaseVersion, domain.TaskResult{Summary: "stale"}); err == nil {
		t.Fatal("database routine accepted stale lease generation")
	}
	if err := workerQueue.MarkFailure(ctx, staleTask.ID, workerID, second.LeaseVersion, domain.TaskFailure{Message: "non-retryable", ErrorType: "test", Retryable: false}); err != nil {
		t.Fatalf("registered Worker DLQ transition: %v", err)
	}
	got, err := adminQueue.GetTask(ctx, staleTask.ID)
	if err != nil || got == nil {
		t.Fatalf("read terminal task: task=%v err=%v", got, err)
	}
	if got.Status != domain.TaskStatusDLQ || got.LeaseVersion != second.LeaseVersion || got.Attempt != second.Attempt || got.LeasedBy != nil || got.LeaseExpiresAt != nil {
		t.Fatalf("unexpected terminal task invariants: status=%s lease_version=%d attempt=%d leased_by=%v expiry=%v", got.Status, got.LeaseVersion, got.Attempt, got.LeasedBy, got.LeaseExpiresAt)
	}
	staleEvents, err := adminQueue.GetTaskEvents(ctx, staleTask.ID)
	if err != nil {
		t.Fatalf("read stale-generation task events: %v", err)
	}
	var reclaimEvents, dlqEvents int
	for _, event := range staleEvents {
		if event.WorkerID != nil && *event.WorkerID == workerID {
			if event.EventType == domain.EventClaimed {
				reclaimEvents++
			}
			if event.EventType == domain.EventMovedToDLQ {
				dlqEvents++
			}
		}
	}
	if reclaimEvents != 2 || dlqEvents != 1 {
		t.Fatalf("reclaim/DLQ event counts=%d/%d, want 2/1 identity-bound events", reclaimEvents, dlqEvents)
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
