// Package effects contains the durable boundary between at-least-once task
// delivery and side effects outside PostgreSQL. A task state transition alone
// cannot prove whether a provider received a request.
package effects

import (
	"context"
	"time"
)

type Policy string

const (
	PolicyPure               Policy = "pure"
	PolicyIdempotent         Policy = "idempotent"
	PolicyProviderIdempotent Policy = "provider-idempotent"
	PolicyReconcilable       Policy = "reconcilable"
	PolicyNonRetriable       Policy = "non-retriable"
)

type Status string

const (
	StatusIntent           Status = "intent"
	StatusApplied          Status = "applied"
	StatusUnknown          Status = "unknown"
	StatusManualResolution Status = "manual_resolution"
)

// Intent is written before the provider is contacted. Generation belongs to
// the worker lease, not to user input, so late receipts remain evidence for an
// old owner and can never overwrite a newer task completion.
type Intent struct {
	EffectID, TaskID, OperationKey, Kind string
	Policy                               Policy
	RequestHash                          string
	Generation, Attempt                  int
}

type Receipt struct {
	ProviderRequestID, ResponseHash string
}

type Record struct {
	EffectID, TaskID, OperationKey, Kind, Policy, Status         string
	ProviderRequestID, RequestHash, ResponseHash, ResolutionNote string
	Generation, Attempt                                          int
	CreatedAt, ReconciledAt                                      *time.Time
}

// Ledger is intentionally small: adapters own provider calls, while the
// ledger records the durable evidence used for retries and operator review.
type Ledger interface {
	RecordIntent(ctx context.Context, intent Intent) error
	RecordReceipt(ctx context.Context, effectID string, receipt Receipt) error
	MarkUnknown(ctx context.Context, effectID, reason string) error
	MarkManualResolution(ctx context.Context, effectID, reason string) error
}

// ReconciliationLedger exposes only the operator-facing review and
// non-destructive manual-resolution operations.
type ReconciliationLedger interface {
	ListUnresolved(ctx context.Context, limit int) ([]Record, error)
	ResolveManually(ctx context.Context, effectID, reason string) error
}
