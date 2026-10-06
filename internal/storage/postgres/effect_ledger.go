package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fantasticyi/distributed-task-lab/internal/effects"
)

// EffectLedger persists external-effect evidence in the same durable database
// as task ownership. It never changes task state: a stale generation's late
// receipt is retained as audit evidence rather than completing a newer owner.
type EffectLedger struct{ db *sql.DB }

func NewEffectLedger(db *sql.DB) *EffectLedger {
	return &EffectLedger{db: db}
}

func (l *EffectLedger) RecordIntent(ctx context.Context, in effects.Intent) error {
	if in.EffectID == "" || in.TaskID == "" || in.OperationKey == "" || in.Kind == "" || in.Policy == "" || in.Generation <= 0 || in.Attempt <= 0 {
		return fmt.Errorf("effect intent has missing identity or execution metadata")
	}
	_, err := l.db.ExecContext(ctx, `
		INSERT INTO effects (effect_id, task_id, operation_key, kind, policy, status, request_hash, generation, attempt)
		VALUES ($1, $2, $3, $4, $5, 'intent', $6, $7, $8)
		ON CONFLICT (task_id, operation_key, generation) DO UPDATE
		SET attempt = GREATEST(effects.attempt, EXCLUDED.attempt),
		    request_hash = EXCLUDED.request_hash
	`, in.EffectID, in.TaskID, in.OperationKey, in.Kind, string(in.Policy), in.RequestHash, in.Generation, in.Attempt)
	if err != nil {
		return fmt.Errorf("record effect intent: %w", err)
	}
	return nil
}

func (l *EffectLedger) RecordReceipt(ctx context.Context, effectID string, receipt effects.Receipt) error {
	result, err := l.db.ExecContext(ctx, `
		UPDATE effects
		SET status = 'applied', provider_request_id = NULLIF($2, ''), response_hash = NULLIF($3, ''), reconciled_at = clock_timestamp()
		WHERE effect_id = $1 AND status IN ('intent', 'unknown', 'applied')
	`, effectID, receipt.ProviderRequestID, receipt.ResponseHash)
	if err != nil {
		return fmt.Errorf("record effect receipt: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("record effect receipt: %s is not pending", effectID)
	}
	return nil
}

func (l *EffectLedger) MarkUnknown(ctx context.Context, effectID, reason string) error {
	return l.mark(ctx, effectID, "unknown", reason)
}

func (l *EffectLedger) MarkManualResolution(ctx context.Context, effectID, reason string) error {
	return l.mark(ctx, effectID, "manual_resolution", reason)
}

func (l *EffectLedger) mark(ctx context.Context, effectID, status, reason string) error {
	result, err := l.db.ExecContext(ctx, `
		UPDATE effects SET status = $2, resolution_note = NULLIF($3, ''), reconciled_at = clock_timestamp()
		WHERE effect_id = $1 AND status IN ('intent', 'unknown')
	`, effectID, status, reason)
	if err != nil {
		return fmt.Errorf("mark effect %s: %w", status, err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("mark effect %s: %s is not pending", status, effectID)
	}
	return nil
}
