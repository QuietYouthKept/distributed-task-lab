package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

// Receipt is the durable observation made by the simulated external system.
type Receipt struct {
	IdempotencyKey, Operation, Target, Status string
	AppliedAt                                 time.Time
}

// SideEffectAdapter models an external payment/notification provider. A receipt
// file is its durable idempotency store; exclusive create makes Apply safe when
// two at-least-once executions race. Production adapters should provide the
// same Apply/LookupByKey contract against the provider's idempotency API.
type SideEffectAdapter struct{ ReceiptDir string }

func NewSideEffectAdapter(receiptDir string) *SideEffectAdapter {
	return &SideEffectAdapter{ReceiptDir: receiptDir}
}
func (a *SideEffectAdapter) receiptPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(a.ReceiptDir, fmt.Sprintf("%x.json", sum[:]))
}
func (a *SideEffectAdapter) LookupByKey(key string) (*Receipt, error) {
	raw, err := os.ReadFile(a.receiptPath(key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt Receipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}
func (a *SideEffectAdapter) Apply(ctx context.Context, key, operation, target string) (*Receipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(a.ReceiptDir, 0o755); err != nil {
		return nil, err
	}
	receipt := &Receipt{IdempotencyKey: key, Operation: operation, Target: target, Status: "applied", AppliedAt: time.Now().UTC()}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(a.receiptPath(key), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return a.LookupByKey(key)
	}
	if err != nil {
		return nil, err
	}
	_, writeErr := f.Write(raw)
	closeErr := f.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return receipt, nil
}

// SideEffectHandler reconciles an unknown response before applying again.
type SideEffectHandler struct{ adapter *SideEffectAdapter }

func NewSideEffectHandler(receiptDir string) *SideEffectHandler {
	return &SideEffectHandler{adapter: NewSideEffectAdapter(receiptDir)}
}
func (h *SideEffectHandler) Run(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	operation, operationID, target := getString(payload, "operation", ""), getString(payload, "operation_id", ""), getString(payload, "target", "")
	if (operation != "payment" && operation != "notification") || operationID == "" || target == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("operation (payment|notification), operation_id and target are required"), domain.ErrorTypeBadPayload)
	}
	key := operation + ":" + operationID
	if receipt, err := h.adapter.LookupByKey(key); err != nil {
		return nil, fmt.Errorf("lookup receipt: %w", err)
	} else if receipt != nil {
		return sideEffectResult(receipt, true), nil
	}
	if getBool(payload, "simulate_timeout_before_apply", false) {
		return nil, fmt.Errorf("simulated provider timeout before apply")
	}
	receipt, err := h.adapter.Apply(ctx, key, operation, target)
	if err != nil {
		return nil, fmt.Errorf("apply external effect: %w", err)
	}
	if getBool(payload, "simulate_response_loss", false) {
		return nil, fmt.Errorf("provider response lost after durable apply; retry will reconcile receipt")
	}
	return sideEffectResult(receipt, false), nil
}
func sideEffectResult(r *Receipt, reconciled bool) *worker.TaskExecResult {
	return &worker.TaskExecResult{Summary: fmt.Sprintf("%s %s for %s (reconciled=%t)", r.Operation, r.Status, r.Target, reconciled), Artifacts: map[string]string{"receipt_key": r.IdempotencyKey, "receipt_status": r.Status, "reconciled": fmt.Sprintf("%t", reconciled)}, Metrics: map[string]float64{"external_side_effects": 1}}
}
