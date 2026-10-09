package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
)

// testService wires a real Postgres-backed service, skipping if no DB is available.
func testService(t *testing.T) (*TaskService, *postgres.PostgresQueue) {
	t.Helper()

	tdb, err := testutil.EnsureDatabase(testutil.DefaultBaseURL(), "tasklab_app")
	if err != nil {
		t.Skipf("skipping: cannot create PostgreSQL database: %v", err)
	}

	db, err := postgres.NewDB(tdb.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := postgres.RunMigrations(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		tdb.Drop()
	})

	q := postgres.NewPostgresQueue(db)
	return NewTaskService(q), q
}

// TestServiceLifecycle drives a task through the full state machine via the
// service + queue: pending -> running -> (retry) -> pending -> running -> dlq
// -> requeue -> pending -> cancel -> failed.
func TestServiceLifecycle(t *testing.T) {
	svc, q := testService(t)
	ctx := context.Background()

	resp, err := svc.CreateTask(ctx, CreateTaskRequest{
		Type:              domain.TaskTypeRAGIngestion,
		Payload:           domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"execution_mode": "mock", "document_id": "doc-1"}},
		MaxAttempts:       2,
		RetryDelaySeconds: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if resp.Status != domain.TaskStatusPending {
		t.Errorf("status = %q, want pending", resp.Status)
	}

	// Attempt 1: fail (retryable) -> back to pending.
	claimed, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim 1: err=%v n=%d", err, len(claimed))
	}
	if err := q.MarkFailure(ctx, resp.ID, "worker-1", claimed[0].LeaseVersion,
		domain.TaskFailure{Message: "flaky", ErrorType: "execution_error", Retryable: true}); err != nil {
		t.Fatalf("MarkFailure 1: %v", err)
	}
	got, err := svc.GetTask(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != domain.TaskStatusPending {
		t.Errorf("after retry: status = %q, want pending", got.Status)
	}

	// Attempt 2: fail again -> DLQ (max_attempts exhausted). ClaimTasks uses
	// the PostgreSQL clock for eligibility, so poll with a finite deadline
	// instead of guessing when database time has crossed next_run_at.
	claimed2 := claimTaskEventually(t, ctx, q, "worker-1", 3*time.Second)
	if err := q.MarkFailure(ctx, resp.ID, "worker-1", claimed2.LeaseVersion,
		domain.TaskFailure{Message: "flaky", ErrorType: "execution_error", Retryable: true}); err != nil {
		t.Fatalf("MarkFailure 2: %v", err)
	}
	got2, err := svc.GetTask(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetTask 2: %v", err)
	}
	if got2.Status != domain.TaskStatusDLQ {
		t.Errorf("after max attempts: status = %q, want dlq", got2.Status)
	}

	// Requeue -> pending.
	if err := svc.Requeue(ctx, resp.ID, "manual retry"); err != nil {
		t.Fatalf("Requeue: %v", err)
	}
	got3, err := svc.GetTask(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetTask 3: %v", err)
	}
	if got3.Status != domain.TaskStatusPending {
		t.Errorf("after requeue: status = %q, want pending", got3.Status)
	}

	// Cancel -> failed.
	if err := svc.Cancel(ctx, resp.ID, "no longer needed"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got4, err := svc.GetTask(ctx, resp.ID)
	if err != nil {
		t.Fatalf("GetTask 4: %v", err)
	}
	if got4.Status != domain.TaskStatusFailed {
		t.Errorf("after cancel: status = %q, want failed", got4.Status)
	}
}

func claimTaskEventually(t *testing.T, ctx context.Context, q *postgres.PostgresQueue, workerID string, timeout time.Duration) domain.Task {
	t.Helper()

	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		claimed, err := q.ClaimTasks(deadlineCtx, workerID, 5, 30*time.Second)
		if err != nil {
			if deadlineCtx.Err() != nil {
				t.Fatalf("claim before bounded deadline: %v", deadlineCtx.Err())
			}
			t.Fatalf("claim: %v", err)
		}
		if len(claimed) > 0 {
			if len(claimed) != 1 {
				t.Fatalf("claim returned %d tasks, want 1", len(claimed))
			}
			return claimed[0]
		}

		select {
		case <-deadlineCtx.Done():
			t.Fatalf("task was not claimable before bounded deadline: %v", deadlineCtx.Err())
		case <-ticker.C:
		}
	}
}

func TestServiceIdempotentCreate(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	key := "svc-idem-1"
	req := CreateTaskRequest{
		Type:           domain.TaskTypeEval,
		Payload:        domain.TaskPayloadEnvelope{Version: "1", Kind: "eval", Data: map[string]any{"execution_mode": "mock", "eval_set_id": "eval-1", "model_profile": "mock-model"}},
		IdempotencyKey: &key,
	}

	r1, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask 1: %v", err)
	}
	r2, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask 2: %v", err)
	}
	if r1.ID != r2.ID {
		t.Errorf("idempotent create returned different IDs: %s vs %s", r1.ID, r2.ID)
	}
}

func TestServiceValidation(t *testing.T) {
	svc, _ := testService(t)

	if _, err := svc.CreateTask(context.Background(), CreateTaskRequest{Type: "bogus"}); err == nil {
		t.Error("expected error for invalid task type")
	}

	emptyKey := ""
	if _, err := svc.CreateTask(context.Background(), CreateTaskRequest{
		Type:           domain.TaskTypeEval,
		Payload:        domain.TaskPayloadEnvelope{Version: "1", Kind: "eval", Data: map[string]any{"execution_mode": "mock", "eval_set_id": "eval-1", "model_profile": "model-1"}},
		IdempotencyKey: &emptyKey,
	}); err == nil {
		t.Error("expected error for empty idempotency key")
	}
}

func TestDeploymentAdmissionRejectsValidButDisabledModeBeforeEnqueue(t *testing.T) {
	_, q := testService(t)
	svc := NewTaskServiceWithAdmission(q, taskpayload.ProductionExecutionPolicy())
	before, err := q.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateTask(context.Background(), CreateTaskRequest{
		Type:    domain.TaskTypeRAGIngestion,
		Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"execution_mode": "mock", "document_id": "must-not-enqueue"}},
	})
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "task_execution_disabled" {
		t.Fatalf("error=%v, want task_execution_disabled", err)
	}
	after, err := q.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after[domain.TaskStatusPending] != before[domain.TaskStatusPending] {
		t.Fatalf("disabled task was enqueued: pending before=%d after=%d", before[domain.TaskStatusPending], after[domain.TaskStatusPending])
	}
}
