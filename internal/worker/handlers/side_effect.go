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
	"github.com/fantasticyi/distributed-task-lab/internal/effects"
	"github.com/fantasticyi/distributed-task-lab/internal/observability"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
	"github.com/google/uuid"
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
type SideEffectHandler struct {
	adapter *SideEffectAdapter
	ledger  effects.Ledger
}

func NewSideEffectHandler(receiptDir string, ledgers ...effects.Ledger) *SideEffectHandler {
	h := &SideEffectHandler{adapter: NewSideEffectAdapter(receiptDir)}
	if len(ledgers) > 0 {
		h.ledger = ledgers[0]
	}
	return h
}
func (h *SideEffectHandler) Run(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	operation, operationID, target := getString(payload, "operation", ""), getString(payload, "operation_id", ""), getString(payload, "target", "")
	if (operation != "payment" && operation != "notification") || operationID == "" || target == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("operation (payment|notification), operation_id and target are required"), domain.ErrorTypeBadPayload)
	}
	key := operation + ":" + operationID
	effectID, err := h.recordIntent(ctx, key, operation, target)
	if err != nil {
		return nil, err
	}
	if receipt, err := h.adapter.LookupByKey(key); err != nil {
		h.markUnknown(ctx, effectID, "provider receipt lookup failed")
		return nil, fmt.Errorf("lookup receipt: %w", err)
	} else if receipt != nil {
		if err := h.recordReceipt(ctx, effectID, receipt); err != nil {
			return nil, err
		}
		return sideEffectResult(receipt, true), nil
	}
	if getBool(payload, "simulate_timeout_before_apply", false) {
		h.markUnknown(ctx, effectID, "provider timeout before apply")
		return nil, fmt.Errorf("simulated provider timeout before apply")
	}
	receipt, err := h.adapter.Apply(ctx, key, operation, target)
	if err != nil {
		h.markUnknown(ctx, effectID, "provider apply returned an error")
		return nil, fmt.Errorf("apply external effect: %w", err)
	}
	if getBool(payload, "simulate_response_loss", false) {
		h.markUnknown(ctx, effectID, "provider response lost after apply")
		return nil, fmt.Errorf("provider response lost after durable apply; retry will reconcile receipt")
	}
	if err := h.recordReceipt(ctx, effectID, receipt); err != nil {
		return nil, err
	}
	return sideEffectResult(receipt, false), nil
}

func (h *SideEffectHandler) recordIntent(ctx context.Context, operationKey, kind, target string) (string, error) {
	if h.ledger == nil {
		return "", nil
	}
	metadata, ok := worker.ExecutionMetadataFromContext(ctx)
	if !ok || metadata.TaskID == "" || metadata.Generation <= 0 || metadata.Attempt <= 0 {
		return "", fmt.Errorf("external effect requires trusted task execution metadata")
	}
	requestHash := sha256.Sum256([]byte(kind + "\x00" + operationKey + "\x00" + target))
	// Stable identity makes receipt recording idempotent when a provider
	// succeeded but the worker crashed before marking task success.
	effectID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(metadata.TaskID+"\x00"+operationKey+"\x00"+fmt.Sprint(metadata.Generation))).String()
	if err := h.ledger.RecordIntent(ctx, effects.Intent{EffectID: effectID, TaskID: metadata.TaskID,
		OperationKey: operationKey, Kind: kind, Policy: effects.PolicyProviderIdempotent,
		RequestHash: fmt.Sprintf("%x", requestHash[:]), Generation: metadata.Generation, Attempt: metadata.Attempt}); err != nil {
		return "", err
	}
	return effectID, nil
}

func (h *SideEffectHandler) recordReceipt(ctx context.Context, effectID string, receipt *Receipt) error {
	if h.ledger == nil {
		return nil
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("hash effect receipt: %w", err)
	}
	sum := sha256.Sum256(raw)
	return h.ledger.RecordReceipt(ctx, effectID, effects.Receipt{ProviderRequestID: receipt.IdempotencyKey, ResponseHash: fmt.Sprintf("%x", sum[:])})
}

func (h *SideEffectHandler) markUnknown(ctx context.Context, effectID, reason string) {
	if h.ledger != nil && effectID != "" {
		observability.Runtime().EffectUnknown()
		_ = h.ledger.MarkUnknown(ctx, effectID, reason)
	}
}
func sideEffectResult(r *Receipt, reconciled bool) *worker.TaskExecResult {
	return &worker.TaskExecResult{Summary: fmt.Sprintf("%s %s for %s (reconciled=%t)", r.Operation, r.Status, r.Target, reconciled), Artifacts: map[string]string{"receipt_key": r.IdempotencyKey, "receipt_status": r.Status, "reconciled": fmt.Sprintf("%t", reconciled)}, Metrics: map[string]float64{"external_side_effects": 1}}
}
