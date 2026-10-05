package handlers

import (
	"context"
	"testing"
)

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
