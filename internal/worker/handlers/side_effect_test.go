package handlers

import (
	"context"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/effects"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

type ledgerCall struct{ status, effectID string }
type recordingLedger struct {
	intents []effects.Intent
	calls   []ledgerCall
}

func (l *recordingLedger) RecordIntent(_ context.Context, intent effects.Intent) error {
	l.intents = append(l.intents, intent)
	return nil
}
func (l *recordingLedger) RecordReceipt(_ context.Context, id string, _ effects.Receipt) error {
	l.calls = append(l.calls, ledgerCall{"applied", id})
	return nil
}
func (l *recordingLedger) MarkUnknown(_ context.Context, id, _ string) error {
	l.calls = append(l.calls, ledgerCall{"unknown", id})
	return nil
}
func (l *recordingLedger) MarkManualResolution(_ context.Context, id, _ string) error {
	l.calls = append(l.calls, ledgerCall{"manual_resolution", id})
	return nil
}

func TestSideEffectResponseLossReconcilesWithoutSecondApply(t *testing.T) {
	h := NewSideEffectHandler(t.TempDir())
	payload := map[string]any{"operation": "payment", "operation_id": "invoice-42", "target": "customer-7", "simulate_response_loss": true}
	if _, err := h.Run(context.Background(), payload); err == nil {
		t.Fatal("first call must expose unknown outcome")
	}
	payload["simulate_response_loss"] = false
	result, err := h.Run(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifacts["reconciled"] != "true" {
		t.Fatal("retry should look up the original receipt")
	}
	if receipt, err := h.adapter.LookupByKey("payment:invoice-42"); err != nil || receipt == nil {
		t.Fatalf("receipt = %v, %v", receipt, err)
	}
}

func TestSideEffectWritesIntentBeforeUnknownAndReconciles(t *testing.T) {
	ledger := &recordingLedger{}
	h := NewSideEffectHandler(t.TempDir(), ledger)
	ctx := worker.WithExecutionMetadata(context.Background(), worker.ExecutionMetadata{TaskID: "11111111-1111-1111-1111-111111111111", Generation: 3, Attempt: 2})
	payload := map[string]any{"operation": "payment", "operation_id": "invoice-43", "target": "customer-8", "simulate_response_loss": true}
	if _, err := h.Run(ctx, payload); err == nil {
		t.Fatal("first execution must expose unknown provider outcome")
	}
	if len(ledger.intents) != 1 || ledger.intents[0].Generation != 3 || ledger.intents[0].Policy != effects.PolicyProviderIdempotent {
		t.Fatalf("intent = %#v", ledger.intents)
	}
	if len(ledger.calls) != 1 || ledger.calls[0].status != "unknown" {
		t.Fatalf("ledger calls = %#v", ledger.calls)
	}
	payload["simulate_response_loss"] = false
	if _, err := h.Run(ctx, payload); err != nil {
		t.Fatal(err)
	}
	if len(ledger.intents) != 2 || ledger.intents[0].EffectID != ledger.intents[1].EffectID {
		t.Fatalf("effect identity must be stable across a retry: %#v", ledger.intents)
	}
	if got := ledger.calls[len(ledger.calls)-1]; got.status != "applied" || got.effectID != ledger.intents[0].EffectID {
		t.Fatalf("receipt call = %#v", got)
	}
}

func TestSideEffectLedgerRejectsUntrustedExecutionMetadata(t *testing.T) {
	h := NewSideEffectHandler(t.TempDir(), &recordingLedger{})
	_, err := h.Run(context.Background(), map[string]any{"operation": "notification", "operation_id": "n-1", "target": "user-1"})
	if err == nil || err.Error() != "external effect requires trusted task execution metadata" {
		t.Fatalf("err = %v", err)
	}
}
