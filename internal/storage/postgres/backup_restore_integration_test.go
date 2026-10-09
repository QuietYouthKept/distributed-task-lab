package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/effects"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestDatabaseBackupRestoreDrill(t *testing.T) {
	if os.Getenv("RUN_BACKUP_RESTORE_DRILL") != "1" {
		t.Skip("RUN_BACKUP_RESTORE_DRILL=1 is required for the explicit local backup/restore drill")
	}
	if os.Getenv("REQUIRE_TEST_DATABASE") != "1" {
		t.Fatal("REQUIRE_TEST_DATABASE=1 is required")
	}
	container := strings.TrimSpace(os.Getenv("PG_BACKUP_CONTAINER"))
	if container == "" {
		t.Fatal("PG_BACKUP_CONTAINER must name the PostgreSQL container used by this local drill")
	}
	if os.Getenv("REQUIRE_MINIO") != "1" {
		t.Fatal("REQUIRE_MINIO=1 is required to verify the restored artifact reference")
	}

	baseURL := testutil.DefaultBaseURL()
	source, err := testutil.EnsureDatabase(baseURL, "tasklab_backup_source")
	if err != nil {
		t.Fatalf("create isolated source database: %v", err)
	}
	t.Cleanup(source.Drop)
	target, err := testutil.EnsureDatabase(baseURL, "tasklab_backup_restore")
	if err != nil {
		t.Fatalf("create isolated restore database: %v", err)
	}
	t.Cleanup(target.Drop)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sourceDB, err := NewDB(source.URL)
	if err != nil {
		t.Fatalf("connect isolated source database: %v", err)
	}
	t.Cleanup(func() { _ = sourceDB.Close() })
	if err := RunMigrations(sourceDB); err != nil {
		t.Fatalf("migrate isolated source database: %v", err)
	}
	queue := NewPostgresQueue(sourceDB)
	auditContext := contract.WithRequestID(auth.WithPrincipal(ctx, auth.Principal{
		Subject: "backup-restore-drill", Kind: auth.KindService,
		Roles: map[auth.Role]struct{}{auth.RoleAdmin: {}},
	}), "backup-restore-request-0001")
	queueName := "backup-restore-" + uuid.NewString()[:8]
	if err := queue.SetQueueConfig(auditContext, domain.QueueConfig{Name: queueName}); err != nil {
		t.Fatalf("configure source queue: %v", err)
	}
	if err := queue.PauseQueue(auditContext, queueName); err != nil {
		t.Fatalf("create source audit row: %v", err)
	}
	if err := queue.ResumeQueue(auditContext, queueName); err != nil {
		t.Fatalf("resume source queue for recovery task: %v", err)
	}

	endpoint, bucket := os.Getenv("MINIO_ENDPOINT"), os.Getenv("MINIO_BUCKET")
	accessKey, secretKey := os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")
	store, err := artifacts.NewS3Store(artifacts.S3Config{
		Endpoint: endpoint, Bucket: bucket, Prefix: "backup-restore/" + uuid.NewString(),
		AccessKey: accessKey, SecretKey: secretKey, Region: "us-east-1", MaxBytes: 1 << 20,
		AllowHTTP: strings.HasPrefix(endpoint, "http://"),
	})
	if err != nil {
		t.Fatalf("configure artifact verification store: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("verify artifact bucket: %v", err)
	}
	artifactKey := "restore-fixture/" + uuid.NewString() + ".json"
	artifactBody := []byte(`{"backup_restore_fixture":"verified"}`)
	if _, err := store.PutIfAbsent(ctx, artifactKey, artifactBody, "application/json"); err != nil {
		t.Fatalf("write representative recovery artifact: %v", err)
	}

	task := newTask("backup-restore-"+uuid.NewString(), 3)
	task.Queue = queueName
	if _, err := queue.Enqueue(ctx, task); err != nil {
		t.Fatalf("enqueue source recovery task: %v", err)
	}
	if _, err := sourceDB.ExecContext(ctx, `UPDATE tasks SET next_run_at=clock_timestamp()-interval '1 millisecond' WHERE id=$1`, task.ID); err != nil {
		t.Fatalf("make source task immediately claimable: %v", err)
	}
	claimed, err := queue.ClaimTasks(ctx, "backup-restore-worker", 1, time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0].ID != task.ID {
		t.Fatalf("claim source task: claimed=%d err=%v", len(claimed), err)
	}
	result := domain.TaskResult{Summary: "backup restore fixture", Artifacts: map[string]string{"fixture_uri": "artifact://" + artifactKey}}
	if err := queue.MarkSuccess(ctx, task.ID, "backup-restore-worker", claimed[0].LeaseVersion, result); err != nil {
		t.Fatalf("complete source task: %v", err)
	}
	if err := NewEffectLedger(sourceDB).RecordIntent(ctx, effects.Intent{
		EffectID: uuid.NewString(), TaskID: task.ID, OperationKey: "restore-check",
		Kind: "notification", Policy: effects.PolicyIdempotent, RequestHash: strings.Repeat("a", 64),
		Generation: claimed[0].LeaseVersion, Attempt: claimed[0].Attempt,
	}); err != nil {
		t.Fatalf("create source effect row: %v", err)
	}
	beforeCounts, err := recoveryTableCounts(ctx, sourceDB)
	if err != nil {
		t.Fatalf("read source recovery counts: %v", err)
	}

	dumpStarted := time.Now()
	dumpCommand := exec.CommandContext(ctx, "docker", "exec", container, "pg_dump", "-U", "postgres", "--format=custom", "--no-owner", "--no-acl", source.Name)
	var backup bytes.Buffer
	var dumpErrors bytes.Buffer
	dumpCommand.Stdout, dumpCommand.Stderr = &backup, &dumpErrors
	if err := dumpCommand.Run(); err != nil {
		t.Fatalf("pg_dump failed (stderr bytes=%d): %v", dumpErrors.Len(), err)
	}
	dumpDuration := time.Since(dumpStarted)
	backupBytes := backup.Bytes()
	backupHash := sha256.Sum256(backupBytes)

	restoreStarted := time.Now()
	restoreCommand := exec.CommandContext(ctx, "docker", "exec", "-i", container, "pg_restore", "-U", "postgres", "--exit-on-error", "--no-owner", "--no-acl", "--dbname", target.Name)
	restoreCommand.Stdin = bytes.NewReader(backupBytes)
	var restoreOutput bytes.Buffer
	restoreCommand.Stdout, restoreCommand.Stderr = &restoreOutput, &restoreOutput
	if err := restoreCommand.Run(); err != nil {
		t.Fatalf("pg_restore failed (output bytes=%d): %v", restoreOutput.Len(), err)
	}
	restoreDuration := time.Since(restoreStarted)

	restoredDB, err := NewDB(target.URL)
	if err != nil {
		t.Fatalf("connect restored database: %v", err)
	}
	t.Cleanup(func() { _ = restoredDB.Close() })
	afterCounts, err := recoveryTableCounts(ctx, restoredDB)
	if err != nil {
		t.Fatalf("read restored recovery counts: %v", err)
	}
	if !equalCounts(beforeCounts, afterCounts) {
		t.Fatalf("restored recovery counts differ: before=%v after=%v", beforeCounts, afterCounts)
	}
	restoredTask, err := NewPostgresQueue(restoredDB).GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("read restored task: %v", err)
	}
	if restoredTask.Status != domain.TaskStatusSuccess || restoredTask.Attempt != claimed[0].Attempt || restoredTask.LeaseVersion != claimed[0].LeaseVersion || restoredTask.Result == nil || restoredTask.Result.Artifacts["fixture_uri"] != "artifact://"+artifactKey {
		t.Fatalf("restored task state or artifact reference differs from source: status=%s attempt=%d lease_version=%d", restoredTask.Status, restoredTask.Attempt, restoredTask.LeaseVersion)
	}
	restoredArtifact, _, err := store.Get(ctx, artifactKey)
	if err != nil {
		t.Fatalf("read referenced recovery artifact after database restore: %v", err)
	}
	artifactHash := sha256.Sum256(restoredArtifact)
	if !bytes.Equal(restoredArtifact, artifactBody) {
		t.Fatal("referenced artifact content changed across the restore verification")
	}
	var pgVersion string
	if err := restoredDB.QueryRowContext(ctx, `SELECT current_setting('server_version')`).Scan(&pgVersion); err != nil {
		t.Fatalf("read restored PostgreSQL version: %v", err)
	}
	report, err := json.Marshal(map[string]any{
		"result": "PASS", "postgres_version": pgVersion, "backup_format": "custom",
		"backup_bytes": len(backupBytes), "backup_sha256": hex.EncodeToString(backupHash[:]),
		"dump_duration_ms":      float64(dumpDuration) / float64(time.Millisecond),
		"restore_duration_ms":   float64(restoreDuration) / float64(time.Millisecond),
		"recovery_table_counts": afterCounts, "artifact_sha256": hex.EncodeToString(artifactHash[:]),
		"task_state": restoredTask.Status, "task_attempt": restoredTask.Attempt, "task_lease_version": restoredTask.LeaseVersion,
	})
	if err != nil {
		t.Fatalf("marshal restore report: %v", err)
	}
	t.Logf("BACKUP_RESTORE_RESULT %s", report)
}

func recoveryTableCounts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	tables := []string{"tasks", "task_events", "security_audit_events", "effects", "schema_migrations"}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}
	return counts, nil
}

func equalCounts(left, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
