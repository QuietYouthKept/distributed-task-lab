package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
)

// To run these integration tests you need a PostgreSQL instance. The easiest way:
//
//   docker run -d --name tasklab-test -p 5433:5432 //     -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres //     -e POSTGRES_DB=tasklab_test postgres:16-alpine
//
//   DATABASE_URL="postgres://postgres:postgres@localhost:5433/tasklab_test?sslmode=disable" //     go test ./internal/storage/postgres/ -v
//
// Or set TEST_DATABASE_URL.

func setupTestDB(t *testing.T) *PostgresQueue {
	t.Helper()

	tdb, err := testutil.EnsureDatabase(testutil.DefaultBaseURL(), "tasklab_test")
	if err != nil {
		if os.Getenv("REQUIRE_TEST_DATABASE") == "1" {
			t.Fatalf("required PostgreSQL integration database unavailable: %v", err)
		}
		t.Skipf("skipping integration test: cannot create PostgreSQL database: %v", err)
	}

	db, err := NewDB(tdb.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		tdb.Drop()
	})

	return NewPostgresQueue(db)
}

// newTask builds a task with sensible defaults for tests.
func newTask(id string, maxAttempts int) *domain.Task {
	return &domain.Task{
		ID:                     id,
		Type:                   domain.TaskTypeRAGIngestion,
		Payload:                domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-" + id}},
		MaxAttempts:            maxAttempts,
		TimeoutSeconds:         60,
		RetryDelaySeconds:      1,
		RetryBackoffMultiplier: 2.0,
	}
}

// enqueue inserts a task, failing the test on error.
func enqueue(t *testing.T, q *PostgresQueue, task *domain.Task) *domain.Task {
	t.Helper()
	created, err := q.Enqueue(context.Background(), task)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	return created
}

// claimOne claims a single task for workerID, failing the test if none are available.
func claimOne(t *testing.T, q *PostgresQueue, workerID string) domain.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		claimed, err := q.ClaimTasks(ctx, workerID, 5, 30*time.Second)
		if err != nil {
			t.Fatalf("ClaimTasks: %v", err)
		}
		if len(claimed) == 1 {
			return claimed[0]
		}
		if len(claimed) > 1 {
			t.Fatalf("expected 1 claimed task, got %d", len(claimed))
		}
		// This helper is only for tests whose task was just enqueued and should
		// already be due. Never hide an early next_run_at bug behind the polling.
		var due int
		if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status='pending' AND next_run_at <= clock_timestamp()`).Scan(&due); err != nil {
			t.Fatalf("check immediate task eligibility using PostgreSQL clock: %v", err)
		}
		if due == 0 {
			t.Fatalf("expected an immediately due pending task, but PostgreSQL reports none")
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out after 10s trying to claim an already-due task; PostgreSQL still reports %d due task(s)", due)
		case <-ticker.C:
		}
	}
}

// claimOneEventually polls ClaimTasks until PostgreSQL considers a retried task
// eligible. Eligibility intentionally remains a database-clock decision: the
// test only supplies a bounded wall-clock deadline so it cannot hang forever.
func claimOneEventually(t *testing.T, q *PostgresQueue, workerID string, timeout time.Duration) domain.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		claimed, err := q.ClaimTasks(ctx, workerID, 5, 30*time.Second)
		if err != nil {
			t.Fatalf("ClaimTasks while polling for eligible task: %v", err)
		}
		if len(claimed) == 1 {
			return claimed[0]
		}
		if len(claimed) > 1 {
			t.Fatalf("expected at most 1 claimed task while polling, got %d", len(claimed))
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timed out after %s waiting for PostgreSQL to make a task claimable", timeout)
		case <-ticker.C:
		}
	}
}

func databaseNow(t *testing.T, q *PostgresQueue, ctx context.Context) time.Time {
	t.Helper()
	var now time.Time
	if err := q.db.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatalf("read PostgreSQL clock: %v", err)
	}
	return now
}

func assertBackoff(t *testing.T, nextRunAt, updatedAt time.Time, want time.Duration) {
	t.Helper()
	const tolerance = 50 * time.Millisecond
	if got := nextRunAt.Sub(updatedAt); got < want-tolerance || got > want+tolerance {
		t.Errorf("backoff = %v, want %v +/- %v", got, want, tolerance)
	}
}

func TestEnqueueAndGetTask(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	created := enqueue(t, q, newTask("test-task-001", 3))

	if created.Status != domain.TaskStatusPending {
		t.Errorf("Status = %q, want pending", created.Status)
	}
	if created.Attempt != 0 {
		t.Errorf("Attempt = %d, want 0", created.Attempt)
	}

	got, err := q.GetTask(ctx, "test-task-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil")
	}
	if got.Status != domain.TaskStatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
}

func TestExpiredFinalCrashMovesTaskToDLQWithoutExtraClaim(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	enqueue(t, q, newTask("test-hard-crash-budget", 1))
	claimed := claimOne(t, q, "crashed-worker")
	if claimed.Attempt != 1 {
		t.Fatalf("first claim attempt = %d, want 1", claimed.Attempt)
	}
	if _, err := q.db.ExecContext(ctx, `UPDATE tasks SET lease_expires_at = clock_timestamp() - interval '1 second' WHERE id = $1`, claimed.ID); err != nil {
		t.Fatal(err)
	}

	reclaimed, err := q.ClaimTasks(ctx, "replacement-worker", 1, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("exhausted task was reclaimed: %+v", reclaimed)
	}
	got, err := q.GetTask(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusDLQ || got.Attempt != 1 {
		t.Fatalf("final state = %s attempt=%d, want dlq attempt=1", got.Status, got.Attempt)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	q := setupTestDB(t)
	got, err := q.GetTask(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent task, got %+v", got)
	}
}

func TestListTasks(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-list-001", 3))
	enqueue(t, q, newTask("test-list-002", 2))

	tasks, err := q.ListTasks(ctx, domain.TaskFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) < 2 {
		t.Fatalf("expected at least 2 tasks, got %d", len(tasks))
	}

	pending := domain.TaskStatusPending
	tasks, err = q.ListTasks(ctx, domain.TaskFilter{Status: &pending, Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks by status: %v", err)
	}
	for _, task := range tasks {
		if task.Status != domain.TaskStatusPending {
			t.Errorf("expected pending, got %s", task.Status)
		}
	}

	queueName := "queue-filter-target"
	queued := newTask("test-list-queue", 3)
	queued.Queue = queueName
	enqueue(t, q, queued)
	tasks, err = q.ListTasks(ctx, domain.TaskFilter{Queue: &queueName, Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks by queue: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != queued.ID {
		t.Fatalf("queue filter returned %+v, want only %s", tasks, queued.ID)
	}
}

func TestListTasksKeysetPaginationUsesStableCreatedAtAndIDOrder(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	for _, id := range []string{"page-1", "page-2", "page-3", "page-4"} {
		if _, err := q.Enqueue(ctx, newTask(id, 3)); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}
	first, err := q.ListTasks(ctx, domain.TaskFilter{Limit: 3})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("first page length=%d, want 3", len(first))
	}
	last := first[len(first)-1]
	second, err := q.ListTasks(ctx, domain.TaskFilter{Limit: 3, AfterCreatedAt: &last.CreatedAt, AfterID: &last.ID})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second page length=%d, want 1", len(second))
	}
	for _, task := range second {
		for _, previous := range first {
			if task.ID == previous.ID {
				t.Fatalf("task %s was duplicated across keyset pages", task.ID)
			}
		}
	}
}

func TestEnqueueIdempotent(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	key := "idem-key-dup"
	first := newTask("test-idem-a", 3)
	first.IdempotencyKey = &key
	created1 := enqueue(t, q, first)

	second := newTask("test-idem-b", 3)
	second.IdempotencyKey = &key
	created2 := enqueue(t, q, second)

	if created1.ID != created2.ID {
		t.Errorf("idempotent enqueue returned different IDs: %s vs %s", created1.ID, created2.ID)
	}
	if created2.ID != "test-idem-a" {
		t.Errorf("expected original task id test-idem-a, got %s", created2.ID)
	}

	// Only one task should exist for the key.
	got, err := q.GetTask(ctx, "test-idem-a")
	if err != nil || got == nil {
		t.Fatalf("original task missing: %v", err)
	}
	got2, err := q.GetTask(ctx, "test-idem-b")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got2 != nil {
		t.Errorf("duplicate task should not exist, got %+v", got2)
	}
}

func TestClaimTasksSuccess(t *testing.T) {
	q := setupTestDB(t)

	enqueue(t, q, newTask("test-claim-001", 3))

	c := claimOne(t, q, "worker-1")
	if c.Status != domain.TaskStatusRunning {
		t.Errorf("Status = %q, want running", c.Status)
	}
	if c.LeasedBy == nil || *c.LeasedBy != "worker-1" {
		t.Errorf("LeasedBy = %v, want worker-1", c.LeasedBy)
	}
	if c.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", c.Attempt)
	}
	if c.LeaseVersion != 1 {
		t.Errorf("LeaseVersion = %d, want 1", c.LeaseVersion)
	}

	// A second worker should not claim the same task.
	claimed2, err := q.ClaimTasks(context.Background(), "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks 2: %v", err)
	}
	if len(claimed2) != 0 {
		t.Errorf("expected 0 claims (already claimed), got %d", len(claimed2))
	}
}

func TestMarkSuccessWithOwnership(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-mark-success-001", 3))
	c := claimOne(t, q, "worker-1")

	result := domain.TaskResult{Summary: "done", Metrics: map[string]float64{"duration_ms": 150}}
	if err := q.MarkSuccess(ctx, c.ID, "worker-1", c.LeaseVersion, result); err != nil {
		t.Fatalf("MarkSuccess: %v", err)
	}

	got, err := q.GetTask(ctx, "test-mark-success-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != domain.TaskStatusSuccess {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if got.Result == nil || got.Result.Summary != "done" {
		t.Errorf("Result.Summary = %v, want 'done'", got.Result)
	}
}

func TestMarkSuccessWrongWorker(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-wrong-worker-001", 3))
	c := claimOne(t, q, "worker-1")

	err := q.MarkSuccess(ctx, c.ID, "worker-2", c.LeaseVersion, domain.TaskResult{Summary: "hijacked"})
	if err == nil {
		t.Error("expected error when wrong worker marks success")
	}
}

func TestMarkSuccessStaleLeaseVersion(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-fence-001", 3))
	c := claimOne(t, q, "worker-1")

	// A stale worker with an out-of-date fencing token must be rejected.
	err := q.MarkSuccess(ctx, c.ID, "worker-1", c.LeaseVersion+100, domain.TaskResult{Summary: "stale"})
	if err == nil {
		t.Error("expected error for stale lease_version")
	}
}

func TestMarkFailureRetry(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-retry-001", 3))
	c := claimOne(t, q, "worker-1")

	failure := domain.TaskFailure{Message: "temporary error", ErrorType: "execution_error", Retryable: true}
	if err := q.MarkFailure(ctx, c.ID, "worker-1", c.LeaseVersion, failure); err != nil {
		t.Fatalf("MarkFailure: %v", err)
	}

	got, err := q.GetTask(ctx, "test-retry-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != domain.TaskStatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	// Retry eligibility is decided by PostgreSQL time. Comparing it with the
	// test process clock after round trips is both non-authoritative and flaky
	// for the one-second default delay. The persisted DB timestamps must still
	// show that this retry was scheduled after the failure transition.
	if !got.NextRunAt.After(got.UpdatedAt) {
		t.Errorf("NextRunAt = %v, want after DB updated_at %v", got.NextRunAt, got.UpdatedAt)
	}
	if got.LeasedBy != nil {
		t.Errorf("LeasedBy should be nil after retry, got %v", *got.LeasedBy)
	}
	if got.LastError == nil || *got.LastError != "temporary error" {
		t.Errorf("LastError = %v, want 'temporary error'", got.LastError)
	}
}

func TestMarkFailureNonRetryable(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := newTask("test-nonretry-001", 5) // plenty of attempts left
	enqueue(t, q, task)
	c := claimOne(t, q, "worker-1")

	failure := domain.TaskFailure{Message: "bad input", ErrorType: "validation_error", Retryable: false}
	if err := q.MarkFailure(ctx, c.ID, "worker-1", c.LeaseVersion, failure); err != nil {
		t.Fatalf("MarkFailure: %v", err)
	}

	got, err := q.GetTask(ctx, "test-nonretry-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != domain.TaskStatusDLQ {
		t.Errorf("Status = %q, want dlq", got.Status)
	}
	if got.DLQReason == nil || *got.DLQReason != "non-retryable error: bad input" {
		t.Errorf("DLQReason = %v, want 'non-retryable error: bad input'", got.DLQReason)
	}
}

func TestMarkFailureMaxAttemptsExhausted(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := newTask("test-max-attempts-001", 1)
	enqueue(t, q, task)
	c := claimOne(t, q, "worker-1")

	failure := domain.TaskFailure{Message: "fatal error", ErrorType: "execution_error", Retryable: true}
	if err := q.MarkFailure(ctx, c.ID, "worker-1", c.LeaseVersion, failure); err != nil {
		t.Fatalf("MarkFailure: %v", err)
	}

	got, err := q.GetTask(ctx, "test-max-attempts-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != domain.TaskStatusDLQ {
		t.Errorf("Status = %q, want dlq", got.Status)
	}
	if got.DLQReason == nil || *got.DLQReason != "max attempts exceeded: fatal error" {
		t.Errorf("DLQReason = %v, want 'max attempts exceeded: fatal error'", got.DLQReason)
	}
}

func TestRetryBackoff(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := newTask("test-backoff-001", 3)
	task.RetryDelaySeconds = 1
	task.RetryBackoffMultiplier = 2.0
	task.RetryMaxDelaySeconds = 0
	task.RetryJitter = false
	enqueue(t, q, task)

	// Attempt 1: fail -> next_run_at is computed from PostgreSQL time + 1s.
	c1 := claimOne(t, q, "worker-1")
	if err := q.MarkFailure(ctx, c1.ID, "worker-1", c1.LeaseVersion, domain.TaskFailure{Message: "e", ErrorType: "execution_error", Retryable: true}); err != nil {
		t.Fatalf("MarkFailure attempt 1: %v", err)
	}
	got1, err := q.GetTask(ctx, c1.ID)
	if err != nil {
		t.Fatalf("GetTask after attempt 1 failure: %v", err)
	}
	if got1 == nil {
		t.Fatal("GetTask after attempt 1 failure returned nil")
	}
	if got1.Status != domain.TaskStatusPending || got1.Attempt != 1 || got1.LeaseVersion != 1 {
		t.Fatalf("after attempt 1 failure status=%q attempt=%d lease_version=%d, want pending/1/1", got1.Status, got1.Attempt, got1.LeaseVersion)
	}
	assertBackoff(t, got1.NextRunAt, got1.UpdatedAt, time.Second)
	if dbNow := databaseNow(t, q, ctx); !dbNow.Before(got1.NextRunAt) {
		t.Fatalf("retry should still be in the future according to PostgreSQL: db_now=%s next_run_at=%s", dbNow, got1.NextRunAt)
	}

	// A retry must not be claimable before the database clock reaches next_run_at.
	claimed, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks before retry due: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("expected 0 claims before retry due, got %d", len(claimed))
	}

	// Attempt 2: bounded polling waits for PostgreSQL's eligibility decision;
	// it is not a fixed sleep that assumes a particular scheduler latency.
	c2 := claimOneEventually(t, q, "worker-1", 10*time.Second)
	if c2.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", c2.Attempt)
	}
	if c2.LeaseVersion != 2 {
		t.Errorf("LeaseVersion = %d, want 2", c2.LeaseVersion)
	}
	if err := q.MarkFailure(ctx, c2.ID, "worker-1", c2.LeaseVersion, domain.TaskFailure{Message: "e", ErrorType: "execution_error", Retryable: true}); err != nil {
		t.Fatalf("MarkFailure attempt 2: %v", err)
	}
	got2, err := q.GetTask(ctx, c2.ID)
	if err != nil {
		t.Fatalf("GetTask after attempt 2 failure: %v", err)
	}
	if got2 == nil {
		t.Fatal("GetTask after attempt 2 failure returned nil")
	}
	if got2.Status != domain.TaskStatusPending || got2.Attempt != 2 || got2.LeaseVersion != 2 {
		t.Fatalf("after attempt 2 failure status=%q attempt=%d lease_version=%d, want pending/2/2", got2.Status, got2.Attempt, got2.LeaseVersion)
	}
	assertBackoff(t, got2.NextRunAt, got2.UpdatedAt, 2*time.Second)
}

func TestRetryTaskReclaimable(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := newTask("test-retry-reclaim-001", 3)
	task.RetryDelaySeconds = 1
	enqueue(t, q, task)
	c := claimOne(t, q, "worker-1")

	if err := q.MarkFailure(ctx, c.ID, "worker-1", c.LeaseVersion, domain.TaskFailure{Message: "err", ErrorType: "execution_error", Retryable: true}); err != nil {
		t.Fatalf("MarkFailure: %v", err)
	}
	beforeDue, err := q.GetTask(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetTask before retry due: %v", err)
	}
	if beforeDue == nil {
		t.Fatal("GetTask before retry due returned nil")
	}
	if beforeDue.Status != domain.TaskStatusPending || beforeDue.Attempt != 1 || beforeDue.LeaseVersion != 1 {
		t.Fatalf("before retry due status=%q attempt=%d lease_version=%d, want pending/1/1", beforeDue.Status, beforeDue.Attempt, beforeDue.LeaseVersion)
	}
	if dbNow := databaseNow(t, q, ctx); !dbNow.Before(beforeDue.NextRunAt) {
		t.Fatalf("retry should still be in the future according to PostgreSQL: db_now=%s next_run_at=%s", dbNow, beforeDue.NextRunAt)
	}

	// Not claimable yet (next_run_at in the future).
	claimed, err := q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 claims (next_run_at in future), got %d", len(claimed))
	}

	claimedAfterDue := claimOneEventually(t, q, "worker-2", 10*time.Second)
	if claimedAfterDue.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", claimedAfterDue.Attempt)
	}
	if claimedAfterDue.LeaseVersion != 2 {
		t.Errorf("LeaseVersion = %d, want 2", claimedAfterDue.LeaseVersion)
	}
	if claimedAfterDue.ClaimReason != "pending" {
		t.Errorf("ClaimReason = %q, want pending", claimedAfterDue.ClaimReason)
	}
}

func TestMarkFailureWrongWorker(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-fail-wrong-worker-001", 3))
	c := claimOne(t, q, "worker-1")

	failure := domain.TaskFailure{Message: "wrong worker", ErrorType: "execution_error", Retryable: true}
	err := q.MarkFailure(ctx, c.ID, "worker-2", c.LeaseVersion, failure)
	if err == nil {
		t.Error("expected error when wrong worker marks failure")
	}
}

func TestReclaimExpiredLease(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	enqueue(t, q, newTask("test-reclaim-001", 3))

	// Claim with a very short lease.
	claimed, err := q.ClaimTasks(ctx, "worker-1", 5, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claimed))
	}

	time.Sleep(10 * time.Millisecond)

	// Worker 2 reclaims; the fencing token must advance.
	reclaimed, err := q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(reclaimed) != 1 {
		t.Fatalf("expected 1 reclaimed task, got %d", len(reclaimed))
	}
	if reclaimed[0].LeasedBy == nil || *reclaimed[0].LeasedBy != "worker-2" {
		t.Errorf("reclaimed LeasedBy = %v, want worker-2", reclaimed[0].LeasedBy)
	}
	if reclaimed[0].LeaseVersion != 2 {
		t.Errorf("LeaseVersion = %d, want 2", reclaimed[0].LeaseVersion)
	}
}

func TestDLQNotClaimable(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := newTask("test-dlq-claim-001", 1)
	enqueue(t, q, task)
	c := claimOne(t, q, "worker-1")

	q.MarkFailure(ctx, c.ID, "worker-1", c.LeaseVersion, domain.TaskFailure{Message: "fatal", ErrorType: "execution_error", Retryable: true})

	claimed, err := q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 claims (DLQ task), got %d", len(claimed))
	}
}

func TestListDLQ(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := newTask("test-list-dlq-001", 1)
	enqueue(t, q, task)
	c := claimOne(t, q, "worker-1")
	q.MarkFailure(ctx, c.ID, "worker-1", c.LeaseVersion, domain.TaskFailure{Message: "dlq test error", ErrorType: "execution_error", Retryable: true})

	dlqTasks, err := q.ListDLQ(ctx, domain.TaskFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListDLQ: %v", err)
	}
	found := false
	for _, task := range dlqTasks {
		if task.ID == "test-list-dlq-001" {
			found = true
			if task.Status != domain.TaskStatusDLQ {
				t.Errorf("Status = %q, want dlq", task.Status)
			}
			if task.DLQReason == nil {
				t.Error("DLQReason should not be nil")
			}
		}
	}
	if !found {
		t.Error("DLQ task not found in ListDLQ results")
	}
}
