package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
)

func rawMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	tdb, err := testutil.EnsureDatabase(testutil.DefaultBaseURL(), "tasklab_migration")
	if err != nil {
		if os.Getenv("REQUIRE_TEST_DATABASE") == "1" {
			t.Fatalf("required PostgreSQL unavailable: %v", err)
		}
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	db, err := NewDB(tdb.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); tdb.Drop() })
	return db
}

func TestMigrationsFreshAndRerun(t *testing.T) {
	db := rawMigrationDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != int(latestSchemaVersion) {
		t.Fatalf("migration rows=%d want=%d", count, latestSchemaVersion)
	}
	var effectTable *string
	if err := db.QueryRow(`SELECT to_regclass('public.effects')::text`).Scan(&effectTable); err != nil || effectTable == nil {
		t.Fatalf("effects table missing after migration: table=%v err=%v", effectTable, err)
	}
	var auditTable *string
	if err := db.QueryRow(`SELECT to_regclass('public.security_audit_events')::text`).Scan(&auditTable); err != nil || auditTable == nil {
		t.Fatalf("security audit table missing after migration: table=%v err=%v", auditTable, err)
	}
	for _, index := range []string{"idx_tasks_created_at_id", "idx_tasks_creator_created_at_id"} {
		var name *string
		if err := db.QueryRow(`SELECT to_regclass($1)::text`, "public."+index).Scan(&name); err != nil || name == nil {
			t.Fatalf("keyset pagination index %s missing: index=%v err=%v", index, name, err)
		}
	}
}

func TestMigrationsUpgradePreviouslyUntrackedSchema(t *testing.T) {
	db := rawMigrationDB(t)
	for _, name := range []string{"001_create_tasks.sql", "002_create_task_events.sql", "003_create_workers.sql", "004_production_hardening.sql", "005_queue_governance.sql", "006_queue_rate_limit.sql"} {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var version int64
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != latestSchemaVersion {
		t.Fatalf("version=%d", version)
	}
}

func TestMigrationsConcurrentProcesses(t *testing.T) {
	db := rawMigrationDB(t)
	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errCh <- RunMigrations(db) }()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent migration: %v", err)
		}
	}
}

func TestMigrationLockTimeoutAndRecovery(t *testing.T) {
	db := rawMigrationDB(t)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	err = RunMigrationsContext(ctx, db)
	cancel()
	if err == nil {
		t.Fatal("expected migration lock timeout")
	}
	if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("recovery rerun: %v", err)
	}
}

func TestMigrationChecksumMismatch(t *testing.T) {
	db := rawMigrationDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum='tampered' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestMigrationHistoryGapFailsClosed(t *testing.T) {
	db := rawMigrationDB(t)
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=3`); err != nil {
		t.Fatal(err)
	}
	err := RunMigrations(db)
	if err == nil || !strings.Contains(err.Error(), "migration history gap") {
		t.Fatalf("gap error=%v", err)
	}
}

func TestMigrationStatementFailureRollsBackWholeFile(t *testing.T) {
	db := rawMigrationDB(t)
	body, err := migrationsFS.ReadFile("migrations/001_create_tasks.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, execErr := tx.Exec(string(body) + "; SELECT definitely_missing_migration_function();")
	if execErr == nil {
		t.Fatal("injected halfway failure unexpectedly succeeded")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var tasksTable *string
	if err := db.QueryRow(`SELECT to_regclass('public.tasks')::text`).Scan(&tasksTable); err != nil {
		t.Fatal(err)
	}
	if tasksTable != nil {
		t.Fatalf("transactional migration left tasks table behind: %s", *tasksTable)
	}
}
