package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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
	if err := ledger.RecordReceipt(ctx, oldID, effects.Receipt{ProviderRequestID: "provider-1", ResponseHash: "response-v1"}); err != nil {
		t.Fatalf("identical provider receipt should be idempotent: %v", err)
	}
	if err := ledger.RecordReceipt(ctx, oldID, effects.Receipt{ProviderRequestID: "provider-2", ResponseHash: "response-mutated"}); err == nil {
		t.Fatal("conflicting receipt unexpectedly overwrote applied evidence")
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
	retry.Attempt = 2
	if err := ledger.RecordIntent(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	mutatedFingerprint := retry
	mutatedFingerprint.RequestHash = "b"
	if err := ledger.RecordIntent(context.Background(), mutatedFingerprint); err == nil {
		t.Fatal("same generation accepted a changed request fingerprint")
	}
	mutatedEffectID := retry
	mutatedEffectID.EffectID = uuid.NewString()
	if err := ledger.RecordIntent(context.Background(), mutatedEffectID); err == nil {
		t.Fatal("same generation accepted a changed effect ID")
	}
	var count, attempt int
	var requestHash string
	var effectID string
	if err := q.db.QueryRow(`SELECT COUNT(*), MAX(attempt), MAX(request_hash), MAX(effect_id::text) FROM effects WHERE task_id = $1`, created.ID).Scan(&count, &attempt, &requestHash, &effectID); err != nil {
		t.Fatal(err)
	}
	if count != 1 || attempt != 2 || requestHash != "a" || effectID != first.EffectID {
		t.Fatalf("count/attempt/hash/effect_id = %d/%d/%s/%s", count, attempt, requestHash, effectID)
	}
}

func TestEffectLedgerReconcilesProviderResponseLossWithoutChangingIdentity(t *testing.T) {
	q := setupTestDB(t)
	task := enqueue(t, q, newTask("effect-provider-reconcile", 3))
	ledger := NewEffectLedger(q.db)
	const idempotencyKey = "payment:invoice-provider-reconcile"
	const operationID = "invoice-provider-reconcile"
	const responseHash = "provider-response-v1"
	effectID := uuid.NewString()
	intent := effects.Intent{EffectID: effectID, TaskID: task.ID, OperationKey: idempotencyKey,
		Kind: "payment", Policy: effects.PolicyProviderIdempotent, RequestHash: "request-fingerprint-v1", Generation: 1, Attempt: 1}
	if err := ledger.RecordIntent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var applied *effects.ProviderReceipt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var request effects.ProviderRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode provider request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.IdempotencyKey != idempotencyKey || request.OperationID != operationID || request.TaskID != task.ID || request.Generation != 1 {
				t.Errorf("provider request identity = %+v", request)
			}
			mu.Lock()
			applied = &effects.ProviderReceipt{OperationID: operationID, ReceiptID: "provider-receipt-1", Status: "completed"}
			mu.Unlock()
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server does not support response-loss injection")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack provider response: %v", err)
				return
			}
			_ = conn.Close()
		case http.MethodGet:
			if r.Header.Get("Idempotency-Key") != idempotencyKey {
				t.Errorf("reconciliation key = %q", r.Header.Get("Idempotency-Key"))
			}
			mu.Lock()
			receipt := applied
			mu.Unlock()
			if receipt == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				State   effects.ReconciliationState `json:"state"`
				Receipt effects.ProviderReceipt     `json:"receipt"`
			}{State: effects.ReconciliationCompleted, Receipt: *receipt})
		default:
			t.Errorf("unexpected provider method %q", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	provider, err := effects.NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := effects.ProviderRequest{OperationID: operationID, IdempotencyKey: idempotencyKey, RequestID: "request-provider-reconcile",
		TaskID: task.ID, Generation: 1, Attempt: 1, Operation: "payment", Target: "customer-1", Payload: json.RawMessage(`{"amount":100}`)}
	apply := provider.Apply(context.Background(), request)
	if apply.Outcome != effects.ProviderUnknown || apply.Err == nil {
		t.Fatalf("dropped response after provider apply = %+v; want unknown outcome", apply)
	}
	if err := ledger.MarkUnknown(context.Background(), effectID, "provider response was lost after request transmission"); err != nil {
		t.Fatal(err)
	}
	reconciled := provider.Reconcile(context.Background(), idempotencyKey)
	if reconciled.State != effects.ReconciliationCompleted || reconciled.Receipt == nil || reconciled.Receipt.ReceiptID != "provider-receipt-1" {
		t.Fatalf("provider reconciliation = %+v", reconciled)
	}

	request.Attempt = 2
	intent.Attempt = 2
	if err := ledger.RecordIntent(context.Background(), intent); err != nil {
		t.Fatalf("same request identity on retry: %v", err)
	}
	responseDigest := sha256.Sum256([]byte(responseHash))
	receipt := effects.Receipt{ProviderRequestID: reconciled.Receipt.ReceiptID, ResponseHash: fmt.Sprintf("%x", responseDigest[:])}
	if err := ledger.RecordReceipt(context.Background(), effectID, receipt); err != nil {
		t.Fatal(err)
	}
	var status, storedRequestHash, storedResponseHash string
	var storedAttempt int
	if err := q.db.QueryRow(`SELECT status, request_hash, response_hash, attempt FROM effects WHERE effect_id=$1`, effectID).Scan(&status, &storedRequestHash, &storedResponseHash, &storedAttempt); err != nil {
		t.Fatal(err)
	}
	if status != string(effects.StatusApplied) || storedRequestHash != "request-fingerprint-v1" || storedResponseHash != receipt.ResponseHash || storedAttempt != 2 {
		t.Fatalf("reconciled ledger state = %s/%s/%s/attempt-%d", status, storedRequestHash, storedResponseHash, storedAttempt)
	}
}

func TestEffectLedgerReceiptAndManualResolutionAreMutuallyExclusive(t *testing.T) {
	q := setupTestDB(t)
	task := enqueue(t, q, newTask("effect-resolution-race", 3))
	ledger := NewEffectLedger(q.db)
	operator := contract.WithRequestID(auth.WithPrincipal(context.Background(), auth.Principal{
		Subject: "operator-race", Kind: auth.KindHuman, Roles: map[auth.Role]struct{}{auth.RoleAdmin: {}},
	}), "request-effect-resolution-race")
	for i := 0; i < 8; i++ {
		effectID := uuid.NewString()
		intent := effects.Intent{EffectID: effectID, TaskID: task.ID, OperationKey: fmt.Sprintf("payment:race-%d", i),
			Kind: "payment", Policy: effects.PolicyProviderIdempotent, RequestHash: fmt.Sprintf("request-%d", i), Generation: 1, Attempt: 1}
		if err := ledger.RecordIntent(context.Background(), intent); err != nil {
			t.Fatal(err)
		}
		if err := ledger.MarkUnknown(context.Background(), effectID, "provider response unknown"); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- ledger.RecordReceipt(context.Background(), effectID, effects.Receipt{ProviderRequestID: "provider-receipt", ResponseHash: fmt.Sprintf("response-%d", i)})
		}()
		go func() {
			<-start
			results <- ledger.ResolveManually(operator, effectID, "operator verified provider state")
		}()
		close(start)
		first, second := <-results, <-results
		if (first == nil) == (second == nil) {
			t.Fatalf("round %d: expected exactly one receipt/manual resolution winner, got errors %v / %v", i, first, second)
		}
		var status, responseHash string
		if err := q.db.QueryRow(`SELECT status, COALESCE(response_hash,'') FROM effects WHERE effect_id=$1`, effectID).Scan(&status, &responseHash); err != nil {
			t.Fatal(err)
		}
		switch status {
		case string(effects.StatusApplied):
			if responseHash != fmt.Sprintf("response-%d", i) {
				t.Fatalf("round %d applied response hash = %q", i, responseHash)
			}
		case string(effects.StatusManualResolution):
			var auditCount int
			if err := q.db.QueryRow(`SELECT COUNT(*) FROM security_audit_events WHERE resource_id=$1 AND action='effect.manual_resolution'`, effectID).Scan(&auditCount); err != nil {
				t.Fatal(err)
			}
			if auditCount != 1 {
				t.Fatalf("round %d manual resolution audit count = %d", i, auditCount)
			}
		default:
			t.Fatalf("round %d terminal effect status = %q", i, status)
		}
	}
}
