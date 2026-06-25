package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/yourusername/distributed-task-lab/internal/domain"
	"github.com/yourusername/distributed-task-lab/internal/queue"
)

// Ensure PostgresQueue implements queue.Queue.
var _ queue.Queue = (*PostgresQueue)(nil)

// PostgresQueue implements queue.Queue using PostgreSQL.
type PostgresQueue struct {
	db *sql.DB
}

// NewPostgresQueue creates a new PostgreSQL-backed queue.
func NewPostgresQueue(db *sql.DB) *PostgresQueue {
	return &PostgresQueue{db: db}
}

// Enqueue inserts a new task in pending state.
func (q *PostgresQueue) Enqueue(ctx context.Context, task *domain.Task) error {
	payloadJSON, err := json.Marshal(task.Payload)
	if err != nil {
		return fmt.Errorf("enqueue: marshal payload: %w", err)
	}

	now := time.Now().UTC()
	task.Status = domain.TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	task.Attempt = 0

	if task.NextRunAt.IsZero() {
		task.NextRunAt = now
	}

	_, err = q.db.ExecContext(ctx, `
		INSERT INTO tasks (
			id, type, status, payload, priority,
			max_attempts, attempt, timeout_seconds, retry_delay_seconds,
			next_run_at, created_at, updated_at, idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12, $13
		)
	`,
		task.ID, string(task.Type), string(task.Status), payloadJSON, task.Priority,
		task.MaxAttempts, task.Attempt, task.TimeoutSeconds, task.RetryDelaySeconds,
		task.NextRunAt, task.CreatedAt, task.UpdatedAt, task.IdempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("enqueue: insert: %w", err)
	}
	q.recordEvent(ctx, task.ID, domain.EventSubmitted, "", "task submitted", map[string]any{"type": string(task.Type)})
	return nil
}
func (q *PostgresQueue) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	row := q.db.QueryRowContext(ctx, `
		SELECT
			id, type, status, payload, result,
			priority, max_attempts, attempt, timeout_seconds, retry_delay_seconds,
			leased_by, lease_expires_at,
			next_run_at, created_at, updated_at, started_at, finished_at,
			last_error, dlq_reason, idempotency_key
		FROM tasks WHERE id = $1
	`, taskID)

	return scanTask(row)
}

// ListTasks retrieves tasks matching the filter.
func (q *PostgresQueue) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	query := `
		SELECT
			id, type, status, payload, result,
			priority, max_attempts, attempt, timeout_seconds, retry_delay_seconds,
			leased_by, lease_expires_at,
			next_run_at, created_at, updated_at, started_at, finished_at,
			last_error, dlq_reason, idempotency_key
		FROM tasks WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, string(*filter.Status))
		argIdx++
	}
	if filter.Type != nil {
		query += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, string(*filter.Type))
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filter.Limit)
		argIdx++
	} else {
		query += " LIMIT 100"
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filter.Offset)
		argIdx++
	}

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: query: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		task, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("list tasks: scan: %w", err)
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

// ClaimTasks atomically claims tasks for a worker using a transaction
// with row-level locking (FOR UPDATE SKIP LOCKED).
//
// It claims tasks that are:
//   - status = 'pending' AND next_run_at <= now, OR
//   - status = 'running' AND lease_expires_at < now (expired lease — worker crash recovery)
func (q *PostgresQueue) ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("claim: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	leaseExpires := now.Add(leaseTimeout)

	rows, err := tx.QueryContext(ctx, `
		SELECT
			id, type, status, payload, result,
			priority, max_attempts, attempt, timeout_seconds, retry_delay_seconds,
			leased_by, lease_expires_at,
			next_run_at, created_at, updated_at, started_at, finished_at,
			last_error, dlq_reason, idempotency_key
		FROM tasks
		WHERE
			(
				(status = 'pending' AND next_run_at <= $1)
				OR
				(status = 'running' AND lease_expires_at < $1)
			)
		ORDER BY priority DESC, created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("claim: select: %w", err)
	}
	defer rows.Close()

	var claimed []domain.Task
	for rows.Next() {
		task, err := scanTaskFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("claim: scan: %w", err)
		}
		claimed = append(claimed, *task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim: rows: %w", err)
	}

	if len(claimed) == 0 {
		return nil, nil
	}

	// Update claimed tasks atomically.
	nowTime := time.Now().UTC()
	for i := range claimed {
		_, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'running',
			    leased_by = $1,
			    lease_expires_at = $2,
			    started_at = COALESCE(started_at, $3),
			    updated_at = $3,
			    attempt = attempt + 1
			WHERE id = $4
		`, workerID, leaseExpires, nowTime, claimed[i].ID)
		if err != nil {
			return nil, fmt.Errorf("claim: update task %s: %w", claimed[i].ID, err)
		}

		claimed[i].Status = domain.TaskStatusRunning
		claimed[i].LeasedBy = &workerID
		claimed[i].LeaseExpiresAt = &leaseExpires
		claimed[i].UpdatedAt = nowTime
		if claimed[i].StartedAt == nil {
			claimed[i].StartedAt = &nowTime
		}
		claimed[i].Attempt++
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("claim: commit: %w", err)
	}

	for _, t := range claimed {
		q.recordEvent(ctx, t.ID, domain.EventClaimed, workerID, "task claimed", map[string]any{"attempt": t.Attempt})
	}

	return claimed, nil
}

// MarkSuccess marks a task as successfully completed.
// Ownership check: only updates if leased_by matches workerID and status is running.
func (q *PostgresQueue) MarkSuccess(ctx context.Context, taskID string, workerID string, result domain.TaskResult) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("mark success: marshal result: %w", err)
	}

	now := time.Now().UTC()
	res, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'success',
		    result = $1,
		    finished_at = $2,
		    updated_at = $2,
		    leased_by = NULL,
		    lease_expires_at = NULL
		WHERE id = $3
		  AND status = 'running'
		  AND leased_by = $4
	`, resultJSON, now, taskID, workerID)
	if err != nil {
		return fmt.Errorf("mark success: update: %w", err)
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("mark success: task %s: %w", taskID, domain.ErrTaskNotClaimed)
	}
	q.recordEvent(ctx, taskID, domain.EventSucceeded, workerID, "task completed", map[string]any{"summary": result.Summary})
	return nil
}
// Ownership check: only updates if leased_by matches workerID and status is running.
//
// Retry behavior:
//   - If attempt < max_attempts: status → pending, next_run_at = now + retry_delay, lease cleared
//   - If attempt >= max_attempts: status → failed (DLQ comes in Phase 4)
func (q *PostgresQueue) MarkFailure(ctx context.Context, taskID string, workerID string, failure domain.TaskFailure) error {
	// Fetch current task state for retry decision.
	task, err := q.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("mark failure: get task: %w", err)
	}
	if task == nil {
		return domain.ErrTaskNotFound
	}

	now := time.Now().UTC()

	if task.Attempt >= task.MaxAttempts {
		// Max attempts exhausted — move to DLQ.
		dlqReason := "max attempts exceeded: " + failure.Message
		res, err := q.db.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'dlq',
			    dlq_reason = $1,
			    last_error = $2,
			    finished_at = $3,
			    updated_at = $3,
			    leased_by = NULL,
			    lease_expires_at = NULL
			WHERE id = $4
			  AND status = 'running'
			  AND leased_by = $5
		`, dlqReason, failure.Message, now, taskID, workerID)
		if err != nil {
			return fmt.Errorf("mark failure: dlq update: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("mark failure: task %s: %w", taskID, domain.ErrTaskNotClaimed)
		}
		q.recordEvent(ctx, taskID, domain.EventMovedToDLQ, workerID, dlqReason, map[string]any{"error": failure.Message})
		return nil
	}

	// Schedule retry: back to pending with delayed next_run_at.
	retryAt := now.Add(time.Duration(task.RetryDelaySeconds) * time.Second)
	res, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'pending',
		    last_error = $1,
		    next_run_at = $2,
		    updated_at = $3,
		    leased_by = NULL,
		    lease_expires_at = NULL
		WHERE id = $4
		  AND status = 'running'
		  AND leased_by = $5
	`, failure.Message, retryAt, now, taskID, workerID)
	if err != nil {
		return fmt.Errorf("mark failure: retry update: %w", err)
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("mark failure: task %s: %w", taskID, domain.ErrTaskNotClaimed)
	}
	q.recordEvent(ctx, taskID, domain.EventRetried, workerID, failure.Message, map[string]any{"next_retry_at": retryAt.Format(time.RFC3339)})
	return nil
}

// ListDLQ retrieves tasks currently in the dead letter queue.
func (q *PostgresQueue) ListDLQ(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	dlqStatus := domain.TaskStatusDLQ
	filter.Status = &dlqStatus
	return q.ListTasks(ctx, filter)
}

// recordEvent inserts a task lifecycle event (fire-and-forget, best-effort).
func (q *PostgresQueue) recordEvent(ctx context.Context, taskID string, eventType domain.TaskEventType, workerID string, message string, metadata map[string]any) {
	metaJSON, _ := json.Marshal(metadata)
	_, _ = q.db.ExecContext(ctx, `
		INSERT INTO task_events (task_id, event_type, worker_id, message, metadata)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)
	`, taskID, string(eventType), workerID, message, metaJSON)
}

// Requeue resets a DLQ task back to pending. Only allowed for dlq tasks.
func (q *PostgresQueue) Requeue(ctx context.Context, taskID string, reason string) error {
	now := time.Now().UTC()
	res, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'pending',
		    attempt = 0,
		    next_run_at = $1,
		    updated_at = $1,
		    last_error = NULL,
		    dlq_reason = NULL,
		    leased_by = NULL,
		    lease_expires_at = NULL,
		    started_at = NULL,
		    finished_at = NULL
		WHERE id = $2 AND status = 'dlq'
	`, now, taskID)
	if err != nil {
		return fmt.Errorf("requeue: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("requeue: task %s not in dlq status", taskID)
	}
	q.recordEvent(ctx, taskID, domain.EventRequeued, "", reason, nil)
	return nil
}

// Cancel marks a task as cancelled. Only pending or running tasks can be cancelled.
func (q *PostgresQueue) Cancel(ctx context.Context, taskID string, reason string) error {
	now := time.Now().UTC()
	res, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'failed',
		    last_error = $1,
		    finished_at = $2,
		    updated_at = $2,
		    leased_by = NULL,
		    lease_expires_at = NULL
		WHERE id = $3
		  AND status IN ('pending', 'running')
	`, "cancelled: "+reason, now, taskID)
	if err != nil {
		return fmt.Errorf("cancel: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cancel: task %s not in cancellable status", taskID)
	}
	q.recordEvent(ctx, taskID, domain.EventCancelled, "", reason, nil)
	return nil
}

// GetTaskEvents retrieves the audit event log for a task.
func (q *PostgresQueue) GetTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, task_id, event_type, COALESCE(worker_id, ''), message, COALESCE(metadata::TEXT, '{}'), created_at
		FROM task_events
		WHERE task_id = $1
		ORDER BY created_at ASC
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task events: query: %w", err)
	}
	defer rows.Close()

	var events []domain.TaskEvent
	for rows.Next() {
		var e domain.TaskEvent
		var workerID, metaStr, createdAt string
		if err := rows.Scan(&e.ID, &e.TaskID, &e.EventType, &workerID, &e.Message, &metaStr, &createdAt); err != nil {
			return nil, fmt.Errorf("get task events: scan: %w", err)
		}
		if workerID != "" {
			e.WorkerID = &workerID
		}
		e.CreatedAt = createdAt
		json.Unmarshal([]byte(metaStr), &e.Metadata)
		events = append(events, e)
	}
	if events == nil {
		events = []domain.TaskEvent{}
	}
	return events, rows.Err()
}

// Heartbeat registers or updates a worker's last_seen_at timestamp.
func (q *PostgresQueue) Heartbeat(ctx context.Context, workerID string) error {
	now := time.Now().UTC()
	host, _ := os.Hostname()
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO workers (worker_id, hostname, last_seen_at, status)
		VALUES ($1, $2, $3, 'active')
		ON CONFLICT (worker_id) DO UPDATE
		SET last_seen_at = $3, hostname = $2
	`, workerID, host, now)
	return err
}

// RenewLease extends the lease on a task if the worker still owns it.
// Returns an error if the lease is no longer valid (another worker claimed it).
func (q *PostgresQueue) RenewLease(ctx context.Context, taskID string, workerID string, leaseTimeout time.Duration) error {
	leaseExpires := time.Now().UTC().Add(leaseTimeout)
	res, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET lease_expires_at = $1, updated_at = $2
		WHERE id = $3 AND status = 'running' AND leased_by = $4
	`, leaseExpires, time.Now().UTC(), taskID, workerID)
	if err != nil {
		return fmt.Errorf("renew lease: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrLeaseExpired
	}
	return nil
}

// DetectStaleWorkers returns worker IDs that haven't sent a heartbeat within the threshold.
func (q *PostgresQueue) DetectStaleWorkers(ctx context.Context, threshold time.Duration) ([]string, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	rows, err := q.db.QueryContext(ctx, `
		SELECT worker_id FROM workers WHERE last_seen_at < $1 AND status = 'active'
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("detect stale workers: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// --- Internal helpers ---

func scanTask(row *sql.Row) (*domain.Task, error) {
	var t domain.Task
	var payloadBytes, resultBytes []byte

	err := row.Scan(
		&t.ID, &t.Type, &t.Status, &payloadBytes, &resultBytes,
		&t.Priority, &t.MaxAttempts, &t.Attempt, &t.TimeoutSeconds, &t.RetryDelaySeconds,
		&t.LeasedBy, &t.LeaseExpiresAt,
		&t.NextRunAt, &t.CreatedAt, &t.UpdatedAt, &t.StartedAt, &t.FinishedAt,
		&t.LastError, &t.DLQReason, &t.IdempotencyKey,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if len(payloadBytes) > 0 {
		json.Unmarshal(payloadBytes, &t.Payload)
	}
	if len(resultBytes) > 0 {
		var res domain.TaskResult
		if json.Unmarshal(resultBytes, &res) == nil {
			t.Result = &res
		}
	}

	return &t, nil
}

func scanTaskFromRows(rows *sql.Rows) (*domain.Task, error) {
	var t domain.Task
	var payloadBytes, resultBytes []byte

	err := rows.Scan(
		&t.ID, &t.Type, &t.Status, &payloadBytes, &resultBytes,
		&t.Priority, &t.MaxAttempts, &t.Attempt, &t.TimeoutSeconds, &t.RetryDelaySeconds,
		&t.LeasedBy, &t.LeaseExpiresAt,
		&t.NextRunAt, &t.CreatedAt, &t.UpdatedAt, &t.StartedAt, &t.FinishedAt,
		&t.LastError, &t.DLQReason, &t.IdempotencyKey,
	)
	if err != nil {
		return nil, err
	}

	if len(payloadBytes) > 0 {
		json.Unmarshal(payloadBytes, &t.Payload)
	}
	if len(resultBytes) > 0 {
		var res domain.TaskResult
		if json.Unmarshal(resultBytes, &res) == nil {
			t.Result = &res
		}
	}

	return &t, nil
}
