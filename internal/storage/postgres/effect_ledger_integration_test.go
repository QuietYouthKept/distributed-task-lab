package postgres

import (
	"context"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/effects"
	"github.com/google/uuid"
)

func TestEffectLedgerKeepsOldGenerationReceiptAsEvidence(t *testing.T) {
	q := setupTestDB(t)
	created := enqueue(t, q, newTask("effect-ledger-task", 3))
	ledger := NewEffectLedger(q.db)
	ctx := context.Background()

	oldID := uuid.NewString()
	old := effects.Intent{EffectID: oldID, TaskID: created.ID, OperationKey: "payment:invoice-9", Kind: "payment", Policy: effects.PolicyProviderIdempotent, RequestHash: "request-v1", Generation: 1, Attempt: 1}
	if err := ledger.RecordIntent(ctx, old); err != nil {
		t.Fatal(err)
	}
	newID := uuid.NewString()
	newer := old
	newer.EffectID, newer.Generation, newer.Attempt, newer.RequestHash = newID, 2, 2, "request-v2"
	if err := ledger.RecordIntent(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if err := ledger.MarkUnknown(ctx, newID, "new owner provider timeout"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordReceipt(ctx, oldID, effects.Receipt{ProviderRequestID: "provider-1", ResponseHash: "response-v1"}); err != nil {
		t.Fatal(err)
	}

	var oldStatus, newStatus, oldResponse string
	if err := q.db.QueryRow(`SELECT status, response_hash FROM effects WHERE effect_id = $1`, oldID).Scan(&oldStatus, &oldResponse); err != nil {
		t.Fatal(err)
	}
	if err := q.db.QueryRow(`SELECT status FROM effects WHERE effect_id = $1`, newID).Scan(&newStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != string(effects.StatusApplied) || oldResponse != "response-v1" {
		t.Fatalf("old effect = %s/%s", oldStatus, oldResponse)
	}
	if newStatus != string(effects.StatusUnknown) {
		t.Fatalf("late old receipt changed new generation: %s", newStatus)
	}
}

func TestEffectReconciliationRequiresIdentityAndWritesAtomicAudit(t *testing.T) {
	q := setupTestDB(t)
	task := enqueue(t, q, newTask("effect-reconcile-task", 3))
	ledger := NewEffectLedger(q.db)
	effectID := uuid.NewString()
	if err := ledger.RecordIntent(context.Background(), effects.Intent{EffectID: effectID, TaskID: task.ID, OperationKey: "payment:order-1", Kind: "payment", Policy: effects.PolicyProviderIdempotent, RequestHash: "request-hash", Generation: 1, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.MarkUnknown(context.Background(), effectID, "provider response timed out"); err != nil {
		t.Fatal(err)
	}
	unresolved, err := ledger.ListUnresolved(context.Background(), 100)
	if err != nil || len(unresolved) != 1 || unresolved[0].EffectID != effectID {
		t.Fatalf("unresolved=%+v err=%v", unresolved, err)
	}
	if err := ledger.ResolveManually(context.Background(), effectID, ""); err == nil {
		t.Fatal("manual resolution without authenticated principal unexpectedly succeeded")
	}
	ctx := contract.WithRequestID(auth.WithPrincipal(context.Background(), auth.Principal{
		Subject: "operator-1", Kind: auth.KindHuman, Roles: map[auth.Role]struct{}{auth.RoleAdmin: {}},
	}), "request-effect-resolve")
	if err := ledger.ResolveManually(ctx, effectID, "provider dashboard confirms settled"); err != nil {
		t.Fatal(err)
	}
	unresolved, err = ledger.ListUnresolved(context.Background(), 100)
	if err != nil || len(unresolved) != 0 {
		t.Fatalf("unresolved after manual resolution=%+v err=%v", unresolved, err)
	}
	var status, note, action, actor, requestID string
	if err := q.db.QueryRow(`SELECT status, resolution_note FROM effects WHERE effect_id=$1`, effectID).Scan(&status, &note); err != nil {
		t.Fatal(err)
	}
	if err := q.db.QueryRow(`SELECT action, actor_id, request_id FROM security_audit_events WHERE resource_id=$1`, effectID).Scan(&action, &actor, &requestID); err != nil {
		t.Fatal(err)
	}
	if status != "manual_resolution" || note != "provider dashboard confirms settled" || action != "effect.manual_resolution" || actor != "operator-1" || requestID != "request-effect-resolve" {
		t.Fatalf("effect/audit mismatch: %s %q %s %s %s", status, note, action, actor, requestID)
	}

	failingID := uuid.NewString()
	if err := ledger.RecordIntent(context.Background(), effects.Intent{EffectID: failingID, TaskID: task.ID, OperationKey: "payment:order-2", Kind: "payment", Policy: effects.PolicyProviderIdempotent, RequestHash: "request-hash-2", Generation: 1, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.MarkUnknown(context.Background(), failingID, "provider outcome unknown"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`CREATE FUNCTION reject_effect_resolution_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='effect.manual_resolution' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(`CREATE TRIGGER reject_effect_resolution_audit BEFORE INSERT ON security_audit_events FOR EACH ROW EXECUTE FUNCTION reject_effect_resolution_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = q.db.Exec(`DROP TRIGGER IF EXISTS reject_effect_resolution_audit ON security_audit_events`)
		_, _ = q.db.Exec(`DROP FUNCTION IF EXISTS reject_effect_resolution_audit()`)
	})
	if err := ledger.ResolveManually(ctx, failingID, "provider dashboard confirms settled"); err == nil {
		t.Fatal("resolution unexpectedly succeeded when audit insert failed")
	}
	if err := q.db.QueryRow(`SELECT status FROM effects WHERE effect_id=$1`, failingID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "unknown" {
		t.Fatalf("effect state committed without audit: %s", status)
	}
}

func TestEffectLedgerIntentIsIdempotentPerGeneration(t *testing.T) {
	q := setupTestDB(t)
	created := enqueue(t, q, newTask("effect-ledger-idempotent", 3))
	ledger := NewEffectLedger(q.db)
	first := effects.Intent{EffectID: uuid.NewString(), TaskID: created.ID, OperationKey: "notification:user-2", Kind: "notification", Policy: effects.PolicyProviderIdempotent, RequestHash: "a", Generation: 1, Attempt: 1}
	if err := ledger.RecordIntent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	retry := first
	retry.EffectID, retry.Attempt, retry.RequestHash = uuid.NewString(), 2, "b"
	if err := ledger.RecordIntent(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	var count, attempt int
	var requestHash string
	if err := q.db.QueryRow(`SELECT COUNT(*), MAX(attempt), MAX(request_hash) FROM effects WHERE task_id = $1`, created.ID).Scan(&count, &attempt, &requestHash); err != nil {
		t.Fatal(err)
	}
	if count != 1 || attempt != 2 || requestHash != "b" {
		t.Fatalf("count/attempt/hash = %d/%d/%s", count, attempt, requestHash)
	}
}
