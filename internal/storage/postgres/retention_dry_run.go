package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	retentionDryRunBatchMax = 1000
	retentionDryRunTimeout  = 5 * time.Second
)

type RetentionCandidate struct {
	TaskID     string    `json:"task_id"`
	Status     string    `json:"status"`
	FinishedAt time.Time `json:"finished_at"`
}

// ListRetentionCandidates is an inventory-only query. It never deletes rows.
// Tasks with a result, durable effect, or security audit reference are omitted;
// object references are not normalized in this schema and require manual review.
func ListRetentionCandidates(ctx context.Context, db *sql.DB, finishedBefore time.Time, limit int) ([]RetentionCandidate, error) {
	if db == nil {
		return nil, errors.New("retention dry run requires a database")
	}
	if finishedBefore.IsZero() {
		return nil, errors.New("retention cutoff must be a non-zero time")
	}
	if limit < 1 || limit > retentionDryRunBatchMax {
		return nil, fmt.Errorf("retention dry-run limit must be between 1 and %d", retentionDryRunBatchMax)
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only retention inventory: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL statement_timeout = '5s'`); err != nil {
		return nil, fmt.Errorf("bound retention inventory duration: %w", err)
	}
	var databaseNow time.Time
	if err := tx.QueryRowContext(ctx, `SELECT statement_timestamp()`).Scan(&databaseNow); err != nil {
		return nil, fmt.Errorf("read retention database clock: %w", err)
	}
	if finishedBefore.After(databaseNow) {
		return nil, errors.New("retention cutoff must not be later than the PostgreSQL clock")
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT t.id, t.status, t.finished_at
		FROM tasks AS t
		WHERE t.status IN ('success', 'failed', 'dlq')
		  AND t.finished_at IS NOT NULL
		  AND t.finished_at < $1
		  AND t.result IS NULL
		  AND NOT EXISTS (SELECT 1 FROM effects AS e WHERE e.task_id = t.id)
		  AND NOT EXISTS (
			SELECT 1 FROM security_audit_events AS a
			WHERE a.resource_type = 'task' AND a.resource_id = t.id
		  )
		ORDER BY t.finished_at ASC, t.id ASC
		LIMIT $2
	`, finishedBefore.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("query retention candidates: %w", err)
	}
	defer rows.Close()
	candidates := make([]RetentionCandidate, 0)
	for rows.Next() {
		var candidate RetentionCandidate
		if err := rows.Scan(&candidate.TaskID, &candidate.Status, &candidate.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan retention candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retention candidates: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finish retention inventory: %w", err)
	}
	return candidates, nil
}
