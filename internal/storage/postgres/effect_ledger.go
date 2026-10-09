package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
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

func (l *EffectLedger) ListUnresolved(ctx context.Context, limit int) ([]effects.Record, error) {
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("effect reconciliation limit must be between 1 and 500")
	}
	rows, err := l.db.QueryContext(ctx, `
		SELECT effect_id::text, task_id, operation_key, kind, policy, status,
		       COALESCE(provider_request_id,''), request_hash, COALESCE(response_hash,''),
		       generation, attempt, COALESCE(resolution_note,''), created_at, reconciled_at
		FROM effects WHERE status IN ('intent','unknown')
		ORDER BY created_at ASC, effect_id ASC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list unresolved effects: %w", err)
	}
	defer rows.Close()
	var records []effects.Record
	for rows.Next() {
		var record effects.Record
		var reconciled sql.NullTime
		if err := rows.Scan(&record.EffectID, &record.TaskID, &record.OperationKey, &record.Kind, &record.Policy, &record.Status,
			&record.ProviderRequestID, &record.RequestHash, &record.ResponseHash, &record.Generation, &record.Attempt,
			&record.ResolutionNote, &record.CreatedAt, &reconciled); err != nil {
			return nil, fmt.Errorf("scan unresolved effect: %w", err)
		}
		if reconciled.Valid {
			record.ReconciledAt = &reconciled.Time
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unresolved effects: %w", err)
	}
	return records, nil
}

func (l *EffectLedger) ResolveManually(ctx context.Context, effectID, reason string) error {
	if _, ok := auth.PrincipalFromContext(ctx); !ok {
		return fmt.Errorf("manual effect resolution requires an authenticated principal")
	}
	if strings.TrimSpace(reason) == "" || len(reason) > 1024 || strings.TrimSpace(reason) != reason {
		return fmt.Errorf("manual resolution reason must be 1 to 1024 bytes without surrounding whitespace")
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin effect resolution: %w", err)
	}
	defer tx.Rollback()
	var taskID string
	err = tx.QueryRowContext(ctx, `
		UPDATE effects SET status='manual_resolution', resolution_note=$2, reconciled_at=clock_timestamp()
		WHERE effect_id=$1 AND status IN ('intent','unknown') RETURNING task_id
	`, effectID, reason).Scan(&taskID)
	if err == sql.ErrNoRows {
		return domain.NewError(domain.ErrorKindConflict, "effect_not_unresolved", "effect is not in an unresolved state", nil)
	}
	if err != nil {
		return fmt.Errorf("update effect resolution: %w", err)
	}
	if err := insertSecurityAudit(ctx, tx, "effect.manual_resolution", "effect", effectID, reason); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit effect resolution: %w", err)
	}
	return nil
}
