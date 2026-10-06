package postgres

import (
	"context"
	"testing"

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
