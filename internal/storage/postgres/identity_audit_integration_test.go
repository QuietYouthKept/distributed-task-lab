package postgres

import (
	"context"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestTaskOwnerScopeAndIdempotencyIsolation(t *testing.T) {
	q := setupTestDB(t)
	ownerA := "user-a"
	ownerB := "user-b"
	key := "shared-client-key"
	taskA := newTask("owner-task-a", 2)
	taskA.CreatedBy, taskA.IdempotencyKey = &ownerA, &key
	if _, err := q.Enqueue(context.Background(), taskA); err != nil {
		t.Fatal(err)
	}

	ctxB := auth.WithPrincipal(context.Background(), auth.Principal{Subject: ownerB, Kind: auth.KindHuman, Roles: map[auth.Role]struct{}{auth.RoleOperator: {}}})
	if got, err := q.GetTask(ctxB, taskA.ID); err != nil || got != nil {
		t.Fatalf("cross-owner GetTask leaked resource: task=%v err=%v", got, err)
	}
	listed, err := q.ListTasks(ctxB, domain.TaskFilter{Limit: 100})
	if err != nil || len(listed) != 0 {
		t.Fatalf("cross-owner list: count=%d err=%v", len(listed), err)
	}
	foreignEvents, err := q.GetTaskEvents(ctxB, taskA.ID)
	if err != nil || len(foreignEvents) != 0 {
		t.Fatalf("cross-owner event query leaked data: count=%d err=%v", len(foreignEvents), err)
	}
	taskB := func() *domain.Task {
		task := newTask("owner-task-b", 2)
		task.CreatedBy = &ownerB
		task.IdempotencyKey = &key
		return task
	}()
	createdB, err := q.Enqueue(ctxB, taskB)
	if err != nil {
		t.Fatalf("same idempotency key should be isolated by creator: %v", err)
	}
	if createdB.ID == taskA.ID {
		t.Fatal("different principals unexpectedly shared an idempotent task")
	}
	retryB := newTask("owner-task-b-retry", 2)
	retryB.CreatedBy = &ownerB
	retryB.IdempotencyKey = &key
	reusedB, err := q.Enqueue(ctxB, retryB)
	if err != nil || reusedB.ID != createdB.ID {
		t.Fatalf("same-principal idempotency returned task=%v err=%v, want %s", reusedB, err, createdB.ID)
	}
	ctxA := auth.WithPrincipal(context.Background(), auth.Principal{Subject: ownerA, Kind: auth.KindHuman, Roles: map[auth.Role]struct{}{auth.RoleOperator: {}}})
	if got, err := q.GetTask(ctxA, taskA.ID); err != nil || got == nil || got.CreatedBy == nil || *got.CreatedBy != ownerA {
		t.Fatalf("owner cannot retrieve task: task=%+v err=%v", got, err)
	}
	ownerEvents, err := q.GetTaskEvents(ctxA, taskA.ID)
	if err != nil || len(ownerEvents) != 1 {
		t.Fatalf("owner event query count=%d err=%v, want 1", len(ownerEvents), err)
	}
}

func TestAdministrativeMutationAndAuditAreAtomic(t *testing.T) {
	q := setupTestDB(t)
	owner := "service:operator"
	task := newTask("audit-cancel-task", 2)
	task.CreatedBy = &owner
	enqueue(t, q, task)
	ctx := contract.WithRequestID(auth.WithPrincipal(context.Background(), auth.Principal{
		Subject: owner, Kind: auth.KindService, Roles: map[auth.Role]struct{}{auth.RoleAdmin: {}},
	}), "request-audit-0001")
	if err := q.Cancel(ctx, task.ID, "operator request"); err != nil {
		t.Fatal(err)
	}
	var actor, requestID, action string
	if err := q.db.QueryRowContext(ctx, `SELECT actor_id, request_id, action FROM security_audit_events WHERE resource_id=$1`, task.ID).Scan(&actor, &requestID, &action); err != nil {
		t.Fatal(err)
	}
	if actor != owner || requestID != "request-audit-0001" || action != "task.cancel" {
		t.Fatalf("audit actor=%q request=%q action=%q", actor, requestID, action)
	}
	got, err := q.GetTask(context.Background(), task.ID)
	if err != nil || got == nil || got.Status != domain.TaskStatusFailed {
		t.Fatalf("cancel state=%v err=%v", got, err)
	}
	if _, err := q.db.ExecContext(ctx, `UPDATE security_audit_events SET reason='tampered' WHERE resource_id=$1`, task.ID); err == nil {
		t.Fatal("audit update unexpectedly succeeded")
	}
}

func TestAdministrativeMutationRollsBackWhenAuditInsertFails(t *testing.T) {
	q := setupTestDB(t)
	if _, err := q.db.Exec(`CREATE FUNCTION reject_security_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`CREATE TRIGGER reject_security_audit BEFORE INSERT ON security_audit_events FOR EACH ROW EXECUTE FUNCTION reject_security_audit_insert()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = q.db.Exec(`DROP TRIGGER IF EXISTS reject_security_audit ON security_audit_events`)
		_, _ = q.db.Exec(`DROP FUNCTION IF EXISTS reject_security_audit_insert()`)
	})
	owner := "admin-1"
	ctx := contract.WithRequestID(auth.WithPrincipal(context.Background(), auth.Principal{
		Subject: owner, Kind: auth.KindHuman, Roles: map[auth.Role]struct{}{auth.RoleAdmin: {}},
	}), "request-audit-failure")
	enqueue(t, q, newTask("audit-rollback-task", 1))
	if err := q.PauseQueue(ctx, domain.DefaultQueueName); err == nil {
		t.Fatal("expected audit failure to abort queue pause")
	}
	var paused bool
	if err := q.db.QueryRow(`SELECT paused FROM queues WHERE name=$1`, domain.DefaultQueueName).Scan(&paused); err != nil {
		t.Fatal(err)
	}
	if paused {
		t.Fatal("queue state committed even though audit insert failed")
	}
	var count int
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM security_audit_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("audit rows=%d, want 0", count)
	}
}
