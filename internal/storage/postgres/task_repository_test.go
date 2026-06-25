package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yourusername/distributed-task-lab/internal/domain"
)

// To run these integration tests, you need a PostgreSQL instance.
// The easiest way:
//
//   docker run -d --name tasklab-test -p 5433:5432 \
//     -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres \
//     -e POSTGRES_DB=tasklab_test postgres:16-alpine
//
//   DATABASE_URL="postgres://postgres:postgres@localhost:5433/tasklab_test?sslmode=disable" \
//     go test ./internal/storage/postgres/ -v
//
// Or use the TEST_DATABASE_URL environment variable.

func testDBURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	// Default: expect a test DB on port 5433 to avoid conflict with dev.
	return "postgres://postgres:postgres@localhost:5433/tasklab_test?sslmode=disable"
}

func setupTestDB(t *testing.T) *PostgresQueue {
	t.Helper()

	db, err := NewDB(testDBURL())
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to PostgreSQL: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Clean up existing test data.
	db.Exec("DELETE FROM tasks")

	t.Cleanup(func() {
		db.Exec("DELETE FROM tasks")
		db.Close()
	})

	return NewPostgresQueue(db)
}

func TestEnqueueAndGetTask(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID:             "test-task-001",
		Type:           domain.TaskTypeRAGIngestion,
		Payload:        domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-001"}},
		MaxAttempts:    3,
		TimeoutSeconds: 60,
		RetryDelaySeconds: 5,
	}

	err := q.Enqueue(ctx, task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	got, err := q.GetTask(ctx, "test-task-001")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil")
	}

	if got.Status != domain.TaskStatusPending {
		t.Errorf("Status = %q, want %q", got.Status, domain.TaskStatusPending)
	}
	if got.Attempt != 0 {
		t.Errorf("Attempt = %d, want 0", got.Attempt)
	}
	if got.Type != domain.TaskTypeRAGIngestion {
		t.Errorf("Type = %q, want %q", got.Type, domain.TaskTypeRAGIngestion)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	got, err := q.GetTask(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent task, got %+v", got)
	}
}

func TestListTasks(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	// Create two tasks.
	task1 := &domain.Task{
		ID: "test-list-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-a"}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	task2 := &domain.Task{
		ID: "test-list-002", Type: domain.TaskTypeEval,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "eval", Data: map[string]any{"eval_set_id": "eval-001"}},
		MaxAttempts: 2, TimeoutSeconds: 120, RetryDelaySeconds: 10,
	}

	if err := q.Enqueue(ctx, task1); err != nil {
		t.Fatalf("Enqueue task1: %v", err)
	}
	if err := q.Enqueue(ctx, task2); err != nil {
		t.Fatalf("Enqueue task2: %v", err)
	}

	// List all tasks.
	tasks, err := q.ListTasks(ctx, domain.TaskFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) < 2 {
		t.Fatalf("expected at least 2 tasks, got %d", len(tasks))
	}

	// Filter by status.
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

	// Filter by type.
	ragType := domain.TaskTypeRAGIngestion
	tasks, err = q.ListTasks(ctx, domain.TaskFilter{Type: &ragType, Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks by type: %v", err)
	}
	for _, task := range tasks {
		if task.Type != domain.TaskTypeRAGIngestion {
			t.Errorf("expected rag_ingestion, got %s", task.Type)
		}
	}
}

func TestEnqueueWithIdempotencyKey(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	key := "idem-key-001"
	task := &domain.Task{
		ID: "test-idem-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-idem"}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
		IdempotencyKey: &key,
	}

	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, err := q.GetTask(ctx, "test-idem-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.IdempotencyKey == nil || *got.IdempotencyKey != key {
		t.Errorf("IdempotencyKey = %v, want %q", got.IdempotencyKey, key)
	}
}

func TestClaimTasksSuccess(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	// Enqueue a task.
	task := &domain.Task{
		ID: "test-claim-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-claim"}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Claim the task.
	claimed, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed task, got %d", len(claimed))
	}

	c := claimed[0]
	if c.ID != "test-claim-001" {
		t.Errorf("ID = %q, want %q", c.ID, "test-claim-001")
	}
	if c.Status != domain.TaskStatusRunning {
		t.Errorf("Status = %q, want running", c.Status)
	}
	if c.LeasedBy == nil || *c.LeasedBy != "worker-1" {
		t.Errorf("LeasedBy = %v, want worker-1", c.LeasedBy)
	}
	if c.LeaseExpiresAt == nil {
		t.Error("LeaseExpiresAt should not be nil")
	}
	if c.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", c.Attempt)
	}

	// A second claim should return nothing (task is already running).
	claimed2, err := q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks 2: %v", err)
	}
	if len(claimed2) != 0 {
		t.Errorf("expected 0 claimed tasks (already claimed), got %d", len(claimed2))
	}
}

func TestMarkSuccessWithOwnership(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: "test-mark-success-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	_, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}

	result := domain.TaskResult{Summary: "done", Metrics: map[string]float64{"duration_ms": 150}}
	err = q.MarkSuccess(ctx, "test-mark-success-001", "worker-1", result)
	if err != nil {
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

	task := &domain.Task{
		ID: "test-wrong-worker-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	q.Enqueue(ctx, task)
	q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)

	// Worker 2 tries to mark success — should fail.
	err := q.MarkSuccess(ctx, "test-wrong-worker-001", "worker-2", domain.TaskResult{Summary: "hijacked"})
	if err == nil {
		t.Error("expected error when wrong worker marks success")
	}
}

func TestMarkFailureRetry(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: "test-retry-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	q.Enqueue(ctx, task)
	q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)

	failure := domain.TaskFailure{Message: "temporary error", ErrorType: "execution_error"}
	err := q.MarkFailure(ctx, "test-retry-001", "worker-1", failure)
	if err != nil {
		t.Fatalf("MarkFailure: %v", err)
	}

	got, err := q.GetTask(ctx, "test-retry-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	// Should be back to pending.
	if got.Status != domain.TaskStatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	// next_run_at should be in the future.
	if !got.NextRunAt.After(time.Now().UTC()) {
		t.Errorf("NextRunAt should be in the future, got %v", got.NextRunAt)
	}
	// Lease should be cleared.
	if got.LeasedBy != nil {
		t.Errorf("LeasedBy should be nil after retry, got %v", *got.LeasedBy)
	}
	// Error recorded.
	if got.LastError == nil || *got.LastError != "temporary error" {
		t.Errorf("LastError = %v, want 'temporary error'", got.LastError)
	}
}

func TestMarkFailureMaxAttemptsExhausted(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: "test-max-attempts-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 1, TimeoutSeconds: 60, RetryDelaySeconds: 1,
	}
	q.Enqueue(ctx, task)
	q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)

	failure := domain.TaskFailure{Message: "fatal error", ErrorType: "permanent"}
	err := q.MarkFailure(ctx, "test-max-attempts-001", "worker-1", failure)
	if err != nil {
		t.Fatalf("MarkFailure: %v", err)
	}

	got, err := q.GetTask(ctx, "test-max-attempts-001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	// Attempt >= max_attempts → final fail.
	if got.Status != domain.TaskStatusFailed {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.LastError == nil || *got.LastError != "fatal error" {
		t.Errorf("LastError = %v, want 'fatal error'", got.LastError)
	}
	// Lease should be cleared.
	if got.LeasedBy != nil {
		t.Errorf("LeasedBy should be nil, got %v", *got.LeasedBy)
	}
}

func TestRetryTaskReclaimable(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: "test-retry-reclaim-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 1,
	}
	q.Enqueue(ctx, task)
	q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)

	// Fail → retry to pending with next_run_at = now + 1s.
	q.MarkFailure(ctx, "test-retry-reclaim-001", "worker-1",
		domain.TaskFailure{Message: "err", ErrorType: "execution_error"})

	// Task is pending but next_run_at is 1s in future — should not be claimable yet.
	claimed, err := q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 claims (next_run_at in future), got %d", len(claimed))
	}

	// Wait for retry_delay to pass.
	time.Sleep(1100 * time.Millisecond)

	// Now task should be claimable.
	claimed, err = q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks after delay: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed task after delay, got %d", len(claimed))
	}
	// Attempt should have been incremented again (now at 2).
	if claimed[0].Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", claimed[0].Attempt)
	}
}

func TestMarkFailureWrongWorker(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: "test-fail-wrong-worker-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	q.Enqueue(ctx, task)
	q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)

	// Worker 2 tries to mark failure — should fail (ownership check).
	failure := domain.TaskFailure{Message: "wrong worker", ErrorType: "execution_error"}
	err := q.MarkFailure(ctx, "test-fail-wrong-worker-001", "worker-2", failure)
	if err == nil {
		t.Error("expected error when wrong worker marks failure")
	}
}

func TestReclaimExpiredLease(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	task := &domain.Task{
		ID: "test-reclaim-001", Type: domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{}},
		MaxAttempts: 3, TimeoutSeconds: 60, RetryDelaySeconds: 5,
	}
	q.Enqueue(ctx, task)

	// Claim with very short lease.
	q.ClaimTasks(ctx, "worker-1", 5, 1*time.Millisecond)

	// Wait for lease to expire.
	time.Sleep(10 * time.Millisecond)

	// Worker 2 should be able to reclaim.
	claimed, err := q.ClaimTasks(ctx, "worker-2", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 reclaimed task, got %d", len(claimed))
	}
	if claimed[0].LeasedBy == nil || *claimed[0].LeasedBy != "worker-2" {
		t.Errorf("reclaimed LeasedBy = %v, want worker-2", claimed[0].LeasedBy)
	}
}
