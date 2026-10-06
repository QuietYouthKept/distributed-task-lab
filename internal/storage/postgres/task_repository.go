package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Ensure PostgresQueue implements queue.Queue.
var _ queue.Queue = (*PostgresQueue)(nil)

// PostgresQueue implements queue.Queue using PostgreSQL.
type PostgresQueue struct {
	db               *sql.DB
	operationTimeout time.Duration
}

// NewPostgresQueue creates a new PostgreSQL-backed queue.
func NewPostgresQueue(db *sql.DB) *PostgresQueue {
	return &PostgresQueue{db: db, operationTimeout: 10 * time.Second}
}

func NewPostgresQueueWithTimeout(db *sql.DB, operationTimeout time.Duration) *PostgresQueue {
	q := NewPostgresQueue(db)
	if operationTimeout > 0 {
		q.operationTimeout = operationTimeout
	}
	return q
}

func (q *PostgresQueue) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	if q.operationTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, q.operationTimeout)
}

// taskColumns is the canonical column list used by every SELECT.
const taskColumns = "id, type, queue, status, payload, result, priority, max_attempts, attempt, timeout_seconds, retry_delay_seconds, retry_max_delay_seconds, retry_backoff_multiplier, retry_jitter, leased_by, lease_expires_at, lease_version, next_run_at, created_at, updated_at, started_at, finished_at, last_error, dlq_reason, idempotency_key"

// taskColumnsAliased returns taskColumns with each column prefixed by alias.
func taskColumnsAliased(alias string) string {
	cols := strings.Split(taskColumns, ", ")
	for i, c := range cols {
		cols[i] = alias + "." + c
	}
	return strings.Join(cols, ", ")
}

// Enqueue inserts a new task in pending state. It is idempotent on the task's
// idempotency_key: if a task with the same key already exists, that task is
// returned instead of inserting a duplicate.
func (q *PostgresQueue) Enqueue(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	ctx, span := otel.Tracer("distributed-task-lab/postgres").Start(ctx, "task.enqueue",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("task.type", string(task.Type)), attribute.String("queue", task.Queue)))
	defer span.End()
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	payloadJSON, err := json.Marshal(task.Payload)
	if err != nil {
		return nil, fmt.Errorf("enqueue: marshal payload: %w", err)
	}

	task.Status = domain.TaskStatusPending
	task.Attempt = 0
	if task.Queue == "" {
		task.Queue = domain.DefaultQueueName
	}

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("enqueue: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Queue rows are the referential-integrity and governance authority. Queues
	// are created transactionally on first use so a task can never become an
	// unclaimable orphan.
	if _, err := tx.ExecContext(ctx, `INSERT INTO queues (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, task.Queue); err != nil {
		return nil, fmt.Errorf("enqueue: ensure queue: %w", err)
	}

	var nextRunAt any
	if !task.NextRunAt.IsZero() {
		nextRunAt = task.NextRunAt
	}

	var insertedID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tasks (
			id, type, queue, status, payload, priority,
			max_attempts, attempt, timeout_seconds, retry_delay_seconds,
			retry_max_delay_seconds, retry_backoff_multiplier, retry_jitter,
			next_run_at, created_at, updated_at, idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			COALESCE($14::timestamptz, clock_timestamp()), clock_timestamp(), clock_timestamp(), $15
		)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, status, attempt, next_run_at, created_at, updated_at
	`,
		task.ID, string(task.Type), task.Queue, string(task.Status), payloadJSON, task.Priority,
		task.MaxAttempts, task.Attempt, task.TimeoutSeconds, task.RetryDelaySeconds,
		task.RetryMaxDelaySeconds, task.RetryBackoffMultiplier, task.RetryJitter,
		nextRunAt, task.IdempotencyKey,
	).Scan(&insertedID, &task.Status, &task.Attempt, &task.NextRunAt, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		if task.IdempotencyKey == nil {
			return nil, fmt.Errorf("enqueue: conflict without idempotency key")
		}
		_ = tx.Rollback()
		return q.getTaskByIdempotencyKey(ctx, *task.IdempotencyKey)
	}
	if err != nil {
		return nil, fmt.Errorf("enqueue: insert: %w", err)
	}

	if err := q.insertEvent(ctx, tx, task.ID, domain.EventSubmitted, "", "task submitted", map[string]any{"type": string(task.Type)}); err != nil {
		return nil, fmt.Errorf("enqueue: event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("enqueue: commit: %w", err)
	}
	return task, nil
}

// GetTask retrieves a single task by its ID.
func (q *PostgresQueue) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	row := q.db.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id = $1", taskID)
	return scanTask(row)
}

// getTaskByIdempotencyKey retrieves a task by its idempotency key.
func (q *PostgresQueue) getTaskByIdempotencyKey(ctx context.Context, key string) (*domain.Task, error) {
	row := q.db.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE idempotency_key = $1", key)
	return scanTask(row)
}

// ListTasks retrieves tasks matching the filter.
func (q *PostgresQueue) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	query := "SELECT " + taskColumns + " FROM tasks WHERE 1=1"
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

// ClaimTasks atomically claims tasks for a worker using a transaction with
// row-level locking (FOR UPDATE SKIP LOCKED). Each claim increments the
// lease_version fencing token so stale workers cannot write back results.
func (q *PostgresQueue) ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error) {
	ctx, span := otel.Tracer("distributed-task-lab/postgres").Start(ctx, "claim.transaction",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("worker_id", workerID), attribute.Int("claim.limit", limit)))
	defer span.End()
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	if limit <= 0 || leaseTimeout <= 0 {
		return nil, nil
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("claim: begin tx: %w", err)
	}
	defer tx.Rollback()

	// A hard-crashed worker cannot call MarkFailure. Terminalize an expired
	// final attempt before reclaim selection so a crash cannot exceed max_attempts.
	exhaustedRows, err := tx.QueryContext(ctx, `
		UPDATE tasks
		SET status = 'dlq',
		    dlq_reason = 'max attempts exceeded after worker lease expiry',
		    last_error = 'worker lease expired',
		    finished_at = clock_timestamp(),
		    updated_at = clock_timestamp(),
		    leased_by = NULL,
		    lease_expires_at = NULL
		WHERE status = 'running'
		  AND lease_expires_at < clock_timestamp()
		  AND attempt >= max_attempts
		RETURNING id
	`)
	if err != nil {
		return nil, fmt.Errorf("claim: terminalize exhausted leases: %w", err)
	}
	var exhaustedIDs []string
	for exhaustedRows.Next() {
		var id string
		if err := exhaustedRows.Scan(&id); err != nil {
			exhaustedRows.Close()
			return nil, fmt.Errorf("claim: scan exhausted lease: %w", err)
		}
		exhaustedIDs = append(exhaustedIDs, id)
	}
	if err := exhaustedRows.Close(); err != nil {
		return nil, fmt.Errorf("claim: close exhausted leases: %w", err)
	}
	for _, id := range exhaustedIDs {
		if err := q.insertEvent(ctx, tx, id, domain.EventMovedToDLQ, "", "max attempts exceeded after worker lease expiry", map[string]any{"error_type": "worker_crash"}); err != nil {
			return nil, fmt.Errorf("claim: exhausted lease event: %w", err)
		}
	}

	type queueState struct {
		maxConcurrency int
		rate           float64
		burst          int
		tokens         float64
		rateUpdatedAt  time.Time
		now            time.Time
		running        int
		paused         bool
		agingAfter     int
		agingBoost     int
	}

	// Queue rows are the aggregate admission authorities. Lock eligible queue
	// authorities before task rows and skip an authority already held by another
	// transaction. This preserves strict per-queue admission while preventing a
	// contended queue A from head-of-line blocking independent queue B.
	queueRows, err := tx.QueryContext(ctx, `
		SELECT q.name
		FROM queues q
		JOIN LATERAL (
			SELECT t.status, t.priority, t.created_at,
			       t.priority + CASE WHEN t.status = 'pending'
			           AND q.aging_after_seconds > 0
			           AND t.created_at <= clock_timestamp() - (q.aging_after_seconds * interval '1 second')
			         THEN q.aging_priority_boost ELSE 0 END AS effective_priority
			FROM tasks t
			WHERE t.queue = q.name
			  AND (
				(t.status = 'pending' AND t.next_run_at <= clock_timestamp())
				OR (t.status = 'running' AND t.lease_expires_at < clock_timestamp())
			  )
			  AND (
				t.status = 'running' OR q.max_concurrency = 0 OR
				(SELECT COUNT(*) FROM tasks r WHERE r.queue=q.name AND r.status='running') < q.max_concurrency
			  )
			  AND (
				q.rate_limit_per_second = 0 OR
				LEAST(q.burst::double precision,
				  q.rate_tokens + GREATEST(0, EXTRACT(EPOCH FROM (clock_timestamp()-q.rate_updated_at))) * q.rate_limit_per_second
				) >= 1
			  )
			ORDER BY CASE WHEN t.status='running' THEN 0 ELSE 1 END,
			         effective_priority DESC, t.created_at ASC
			LIMIT 1
		) candidate ON true
		WHERE q.paused = false
		ORDER BY CASE WHEN candidate.status='running' THEN 0 ELSE 1 END,
		         candidate.effective_priority DESC, candidate.created_at ASC, q.name ASC
		LIMIT $1
		FOR UPDATE OF q SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim: select queue authorities: %w", err)
	}
	queueNames := make([]string, 0, limit)
	for queueRows.Next() {
		var name string
		if err := queueRows.Scan(&name); err != nil {
			queueRows.Close()
			return nil, fmt.Errorf("claim: scan queue authority: %w", err)
		}
		queueNames = append(queueNames, name)
	}
	if err := queueRows.Close(); err != nil {
		return nil, fmt.Errorf("claim: close queue authorities: %w", err)
	}
	if len(queueNames) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("claim: commit exhausted leases: %w", err)
		}
		return nil, nil
	}

	rows, err := tx.QueryContext(ctx, `SELECT `+taskColumnsAliased("t")+`
		FROM tasks t
		JOIN queues q ON q.name = t.queue
		WHERE t.queue = ANY($2)
		  AND (
			(t.status = 'pending' AND t.next_run_at <= clock_timestamp())
			OR (t.status = 'running' AND t.lease_expires_at < clock_timestamp())
		  )
		ORDER BY CASE WHEN t.status='running' THEN 0 ELSE 1 END,
		         (t.priority + CASE WHEN t.status = 'pending'
		             AND q.aging_after_seconds > 0
		             AND t.created_at <= clock_timestamp() - (q.aging_after_seconds * interval '1 second')
		           THEN q.aging_priority_boost ELSE 0 END) DESC,
		         t.created_at ASC
		LIMIT $1
		FOR UPDATE OF t SKIP LOCKED
	`, limit, pq.Array(queueNames))
	if err != nil {
		return nil, fmt.Errorf("claim: select tasks: %w", err)
	}

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
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("claim: close rows: %w", err)
	}

	if len(claimed) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("claim: commit exhausted leases: %w", err)
		}
		return nil, nil
	}

	sort.Strings(queueNames)
	states := make(map[string]*queueState, len(queueNames))
	// Queue authorities are already locked by the SKIP LOCKED query above.
	for _, name := range queueNames {
		s := &queueState{}
		if err := tx.QueryRowContext(ctx, `
			SELECT max_concurrency, rate_limit_per_second, burst,
			       rate_tokens, rate_updated_at, clock_timestamp(), paused,
			       aging_after_seconds, aging_priority_boost
			FROM queues WHERE name = $1
		`, name).Scan(&s.maxConcurrency, &s.rate, &s.burst, &s.tokens, &s.rateUpdatedAt, &s.now, &s.paused, &s.agingAfter, &s.agingBoost); err != nil {
			return nil, fmt.Errorf("claim: lock queue %s: %w", name, err)
		}
		// This is intentionally a second SQL statement after the queue lock is
		// acquired. Under READ COMMITTED it receives a fresh snapshot that sees
		// claims committed by the previous queue-lock holder; counting inside the
		// potentially-blocking FOR UPDATE statement could use a pre-wait snapshot
		// and admit beyond max_concurrency.
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE queue = $1 AND status = 'running'`, name).Scan(&s.running); err != nil {
			return nil, fmt.Errorf("claim: count running queue %s: %w", name, err)
		}
		if s.rate > 0 {
			s.tokens = math.Min(float64(s.burst), s.tokens+math.Max(0, s.now.Sub(s.rateUpdatedAt).Seconds())*s.rate)
		}
		states[name] = s
	}

	accepted := make([]domain.Task, 0, len(claimed))
	for i := range claimed {
		if len(accepted) >= limit {
			break
		}
		s := states[claimed[i].Queue]
		if s.paused {
			continue
		}
		isReclaim := claimed[i].Status == domain.TaskStatusRunning
		// A reclaim replaces an existing ownership generation and therefore
		// never consumes another max-concurrency slot.
		if !isReclaim && s.maxConcurrency > 0 && s.running >= s.maxConcurrency {
			continue
		}
		// Cluster-global queue rate is admitted under the same queue row lock.
		if s.rate > 0 && s.tokens < 1 {
			continue
		}
		if s.rate > 0 {
			s.tokens--
		}

		var leaseExpires, updatedAt time.Time
		err := tx.QueryRowContext(ctx, `
			UPDATE tasks
			SET status = 'running',
			    leased_by = $1,
			    lease_expires_at = clock_timestamp() + ($2 * interval '1 microsecond'),
			    lease_version = lease_version + 1,
			    started_at = COALESCE(started_at, clock_timestamp()),
			    updated_at = clock_timestamp(),
			    attempt = attempt + 1
			WHERE id = $3
			RETURNING lease_expires_at, updated_at
		`, workerID, leaseTimeout.Microseconds(), claimed[i].ID).Scan(&leaseExpires, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("claim: update task %s: %w", claimed[i].ID, err)
		}

		claimed[i].Status = domain.TaskStatusRunning
		claimed[i].LeasedBy = &workerID
		claimed[i].LeaseExpiresAt = &leaseExpires
		claimed[i].LeaseVersion++
		claimed[i].UpdatedAt = updatedAt
		if claimed[i].StartedAt == nil {
			claimed[i].StartedAt = &updatedAt
		}
		claimed[i].Attempt++

		reason := "pending"
		if isReclaim {
			reason = "expired_lease"
		}
		claimed[i].ClaimReason = reason
		promoted := !isReclaim && s.agingAfter > 0 && s.agingBoost > 0 && !claimed[i].CreatedAt.Add(time.Duration(s.agingAfter)*time.Second).After(s.now)
		span.AddEvent("task.claimed", trace.WithAttributes(
			attribute.String("task.type", string(claimed[i].Type)),
			attribute.String("queue", claimed[i].Queue),
			attribute.Int("attempt", claimed[i].Attempt),
			attribute.Int("lease_version", claimed[i].LeaseVersion),
			attribute.String("worker_id", workerID),
			attribute.String("claim_reason", reason),
		))
		if promoted {
			if err := q.insertEvent(ctx, tx, claimed[i].ID, domain.EventPriorityPromoted, workerID, "task priority promoted by aging", map[string]any{"original_priority": claimed[i].Priority, "effective_priority": claimed[i].Priority + s.agingBoost, "wait_threshold_seconds": s.agingAfter}); err != nil {
				return nil, fmt.Errorf("claim: priority promotion event: %w", err)
			}
		}
		if err := q.insertEvent(ctx, tx, claimed[i].ID, domain.EventClaimed, workerID, "task claimed", map[string]any{"attempt": claimed[i].Attempt, "lease_version": claimed[i].LeaseVersion, "claim_reason": reason, "priority_promoted": promoted}); err != nil {
			return nil, fmt.Errorf("claim: event: %w", err)
		}
		accepted = append(accepted, claimed[i])
		if !isReclaim {
			s.running++
		}
	}
	for name, s := range states {
		if s.rate > 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE queues SET rate_tokens=$1, rate_updated_at=$2 WHERE name=$3`, s.tokens, s.now, name); err != nil {
				return nil, fmt.Errorf("claim: update rate state %s: %w", name, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("claim: commit: %w", err)
	}
	return accepted, nil
}

// MarkSuccess marks a task as successfully completed in a single transaction.
// The lease_version fencing token must match, preventing stale workers from
// writing results after their lease was reclaimed.
func (q *PostgresQueue) MarkSuccess(ctx context.Context, taskID string, workerID string, leaseVersion int, result domain.TaskResult) error {
	ctx, span := otel.Tracer("distributed-task-lab/postgres").Start(ctx, "task.mark_success",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("task.id", taskID), attribute.String("worker_id", workerID), attribute.Int("lease_version", leaseVersion)))
	defer span.End()
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("mark success: marshal result: %w", err)
	}

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mark success: begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'success',
		    result = $1,
		    finished_at = clock_timestamp(),
		    updated_at = clock_timestamp(),
		    leased_by = NULL,
		    lease_expires_at = NULL
		WHERE id = $2
		  AND status = 'running'
		  AND leased_by = $3
		  AND lease_version = $4
	`, resultJSON, taskID, workerID, leaseVersion)
	if err != nil {
		return fmt.Errorf("mark success: update: %w", err)
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("mark success: task %s: %w", taskID, domain.ErrTaskNotClaimed)
	}
	if err := q.insertEvent(ctx, tx, taskID, domain.EventSucceeded, workerID, "task completed", map[string]any{"summary": result.Summary}); err != nil {
		return fmt.Errorf("mark success: event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mark success: commit: %w", err)
	}
	return nil
}

// MarkFailure handles retry/DLQ in a single transaction. It locks the task row
// (FOR UPDATE) to read the current attempt without a TOCTOU race, then applies
// retry-with-backoff or moves the task to the DLQ, recording the event in the
// same transaction.
func (q *PostgresQueue) MarkFailure(ctx context.Context, taskID string, workerID string, leaseVersion int, failure domain.TaskFailure) error {
	ctx, span := otel.Tracer("distributed-task-lab/postgres").Start(ctx, "task.mark_failure",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("task.id", taskID), attribute.String("worker_id", workerID), attribute.Int("lease_version", leaseVersion)))
	defer span.End()
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mark failure: begin tx: %w", err)
	}
	defer tx.Rollback()

	var attempt, maxAttempts, retryDelaySeconds, retryMaxDelaySeconds int
	var backoffMultiplier float64
	var jitter bool
	err = tx.QueryRowContext(ctx, `
		SELECT attempt, max_attempts, retry_delay_seconds,
		       retry_max_delay_seconds, retry_backoff_multiplier, retry_jitter
		FROM tasks
		WHERE id = $1 AND status = 'running' AND leased_by = $2 AND lease_version = $3
		FOR UPDATE
	`, taskID, workerID, leaseVersion).Scan(&attempt, &maxAttempts, &retryDelaySeconds, &retryMaxDelaySeconds, &backoffMultiplier, &jitter)
	if err == sql.ErrNoRows {
		return fmt.Errorf("mark failure: task %s: %w", taskID, domain.ErrTaskNotClaimed)
	}
	if err != nil {
		return fmt.Errorf("mark failure: get task: %w", err)
	}
	span.SetAttributes(attribute.Int("attempt", attempt))

	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fmt.Errorf("mark failure: database time: %w", err)
	}

	// Non-retryable failures go straight to the DLQ regardless of attempts left.
	if !failure.Retryable {
		dlqReason := "non-retryable error: " + failure.Message
		_, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'dlq',
			    dlq_reason = $1,
			    last_error = $2,
			    finished_at = $3,
			    updated_at = $3,
			    leased_by = NULL,
			    lease_expires_at = NULL
			WHERE id = $4
		`, dlqReason, failure.Message, now, taskID)
		if err != nil {
			return fmt.Errorf("mark failure: dlq update: %w", err)
		}
		if err := q.insertEvent(ctx, tx, taskID, domain.EventMovedToDLQ, workerID, dlqReason, map[string]any{"error": failure.Message, "error_type": failure.ErrorType}); err != nil {
			return fmt.Errorf("mark failure: event: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("mark failure: commit: %w", err)
		}
		return nil
	}

	if attempt >= maxAttempts {
		dlqReason := "max attempts exceeded: " + failure.Message
		_, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'dlq',
			    dlq_reason = $1,
			    last_error = $2,
			    finished_at = $3,
			    updated_at = $3,
			    leased_by = NULL,
			    lease_expires_at = NULL
			WHERE id = $4
		`, dlqReason, failure.Message, now, taskID)
		if err != nil {
			return fmt.Errorf("mark failure: dlq update: %w", err)
		}
		if err := q.insertEvent(ctx, tx, taskID, domain.EventMovedToDLQ, workerID, dlqReason, map[string]any{"error": failure.Message, "error_type": failure.ErrorType}); err != nil {
			return fmt.Errorf("mark failure: event: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("mark failure: commit: %w", err)
		}
		return nil
	}

	// Schedule retry with exponential backoff + jitter.
	t := domain.Task{
		RetryDelaySeconds:      retryDelaySeconds,
		RetryMaxDelaySeconds:   retryMaxDelaySeconds,
		RetryBackoffMultiplier: backoffMultiplier,
		RetryJitter:            jitter,
	}
	retryAt := now.Add(t.RetryDelayForAttempt(attempt))

	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'pending',
		    last_error = $1,
		    next_run_at = $2,
		    updated_at = $3,
		    leased_by = NULL,
		    lease_expires_at = NULL
		WHERE id = $4
	`, failure.Message, retryAt, now, taskID)
	if err != nil {
		return fmt.Errorf("mark failure: retry update: %w", err)
	}

	if err := q.insertEvent(ctx, tx, taskID, domain.EventRetried, workerID, failure.Message, map[string]any{"next_retry_at": retryAt.Format(time.RFC3339), "attempt": attempt, "error_type": failure.ErrorType}); err != nil {
		return fmt.Errorf("mark failure: event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mark failure: commit: %w", err)
	}
	return nil
}

// ListDLQ retrieves tasks currently in the dead letter queue.
func (q *PostgresQueue) ListDLQ(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	dlqStatus := domain.TaskStatusDLQ
	filter.Status = &dlqStatus
	return q.ListTasks(ctx, filter)
}

// QueueDepth returns task counts grouped by queue and status.
func (q *PostgresQueue) QueueDepth(ctx context.Context) (map[string]map[domain.TaskStatus]int64, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT queue, status, COUNT(*) FROM tasks GROUP BY queue, status`)
	if err != nil {
		return nil, fmt.Errorf("queue depth: query: %w", err)
	}
	defer rows.Close()

	depth := map[string]map[domain.TaskStatus]int64{}
	for rows.Next() {
		var queueName, status string
		var count int64
		if err := rows.Scan(&queueName, &status, &count); err != nil {
			return nil, fmt.Errorf("queue depth: scan: %w", err)
		}
		if depth[queueName] == nil {
			depth[queueName] = map[domain.TaskStatus]int64{}
		}
		depth[queueName][domain.TaskStatus(status)] = count
	}
	return depth, rows.Err()
}

// OldestPendingAge returns the age of the oldest pending task (0 if none).
func (q *PostgresQueue) OldestPendingAge(ctx context.Context) (time.Duration, error) {
	var ageSeconds sql.NullFloat64
	if err := q.db.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM (clock_timestamp() - MIN(next_run_at))) FROM tasks WHERE status = 'pending'`).Scan(&ageSeconds); err != nil {
		return 0, fmt.Errorf("oldest pending age: %w", err)
	}
	if !ageSeconds.Valid {
		return 0, nil
	}
	age := time.Duration(ageSeconds.Float64 * float64(time.Second))
	if age < 0 {
		age = 0
	}
	return age, nil
}

// PauseQueue pauses a queue so workers stop claiming its tasks.
func (q *PostgresQueue) PauseQueue(ctx context.Context, name string) error {
	res, err := q.db.ExecContext(ctx, `UPDATE queues SET paused = true, updated_at = clock_timestamp() WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("pause queue: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("pause queue: queue %s not found", name)
	}
	return nil
}

// ResumeQueue resumes a paused queue.
func (q *PostgresQueue) ResumeQueue(ctx context.Context, name string) error {
	res, err := q.db.ExecContext(ctx, `UPDATE queues SET paused = false, updated_at = clock_timestamp() WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("resume queue: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("resume queue: queue %s not found", name)
	}
	return nil
}

// ListQueues returns all queues with their governance config.
func (q *PostgresQueue) ListQueues(ctx context.Context) ([]domain.QueueConfig, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT name, max_concurrency, rate_limit_per_second, burst, aging_after_seconds, aging_priority_boost, paused FROM queues ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	defer rows.Close()

	var queues []domain.QueueConfig
	for rows.Next() {
		var c domain.QueueConfig
		if err := rows.Scan(&c.Name, &c.MaxConcurrency, &c.RateLimitPerSecond, &c.Burst, &c.AgingAfterSeconds, &c.AgingPriorityBoost, &c.Paused); err != nil {
			return nil, fmt.Errorf("list queues: scan: %w", err)
		}
		queues = append(queues, c)
	}
	return queues, rows.Err()
}

// SetQueueConfig upserts a queue's max concurrency and rate limit. It does not
// change the paused flag (use PauseQueue/ResumeQueue for that).
func (q *PostgresQueue) SetQueueConfig(ctx context.Context, config domain.QueueConfig) error {
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	if config.Name == "" || len(config.Name) > 128 {
		return fmt.Errorf("set queue config: invalid queue name")
	}
	if config.MaxConcurrency < 0 || config.RateLimitPerSecond < 0 || config.AgingAfterSeconds < 0 || config.AgingPriorityBoost < 0 {
		return fmt.Errorf("set queue config: negative limits are invalid")
	}
	if config.Burst <= 0 {
		config.Burst = 1
	}
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO queues (name, max_concurrency, rate_limit_per_second, burst, aging_after_seconds, aging_priority_boost, rate_tokens, rate_updated_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, clock_timestamp(), clock_timestamp())
		ON CONFLICT (name) DO UPDATE
		SET max_concurrency = $2,
		    rate_limit_per_second = $3,
		    burst = $4,
		    aging_after_seconds = $5,
		    aging_priority_boost = $6,
		    rate_tokens = CASE
		      WHEN queues.rate_limit_per_second = $3 AND queues.burst = $4 THEN queues.rate_tokens
		      ELSE LEAST(queues.rate_tokens, $7)
		    END,
		    rate_updated_at = CASE
		      WHEN queues.rate_limit_per_second = $3 AND queues.burst = $4 THEN queues.rate_updated_at
		      ELSE clock_timestamp()
		    END,
		    updated_at = clock_timestamp()
	`, config.Name, config.MaxConcurrency, config.RateLimitPerSecond, config.Burst, config.AgingAfterSeconds, config.AgingPriorityBoost, float64(config.Burst))
	if err != nil {
		return fmt.Errorf("set queue config: %w", err)
	}
	return nil
}

// insertEvent inserts a task lifecycle event within the given transaction, so
// the event is committed atomically with the state change it describes.
func (q *PostgresQueue) insertEvent(ctx context.Context, tx *sql.Tx, taskID string, eventType domain.TaskEventType, workerID string, message string, metadata map[string]any) error {
	metaJSON, _ := json.Marshal(metadata)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, event_type, worker_id, message, metadata)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)
	`, taskID, string(eventType), workerID, message, metaJSON)
	return err
}

// Requeue resets a DLQ task back to pending. Only allowed for dlq tasks.
func (q *PostgresQueue) Requeue(ctx context.Context, taskID string, reason string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("requeue: begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'pending',
		    attempt = 0,
		    next_run_at = clock_timestamp(),
		    updated_at = clock_timestamp(),
		    last_error = NULL,
		    dlq_reason = NULL,
		    leased_by = NULL,
		    lease_expires_at = NULL,
		    lease_version = lease_version + 1,
		    started_at = NULL,
		    finished_at = NULL
		WHERE id = $1 AND status = 'dlq'
	`, taskID)
	if err != nil {
		return fmt.Errorf("requeue: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("requeue: task %s not in dlq status", taskID)
	}
	if err := q.insertEvent(ctx, tx, taskID, domain.EventRequeued, "", reason, nil); err != nil {
		return fmt.Errorf("requeue: event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("requeue: commit: %w", err)
	}
	return nil
}

// Cancel marks a task as cancelled. Only pending or running tasks can be cancelled.
func (q *PostgresQueue) Cancel(ctx context.Context, taskID string, reason string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cancel: begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'failed',
		    last_error = $1,
		    finished_at = clock_timestamp(),
		    updated_at = clock_timestamp(),
		    leased_by = NULL,
		    lease_expires_at = NULL,
		    lease_version = lease_version + 1
		WHERE id = $2
		  AND status IN ('pending', 'running')
	`, "cancelled: "+reason, taskID)
	if err != nil {
		return fmt.Errorf("cancel: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cancel: task %s not in cancellable status", taskID)
	}
	if err := q.insertEvent(ctx, tx, taskID, domain.EventCancelled, "", reason, nil); err != nil {
		return fmt.Errorf("cancel: event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cancel: commit: %w", err)
	}
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

// Stats returns task counts grouped by status using a single aggregate query.
func (q *PostgresQueue) Stats(ctx context.Context) (map[domain.TaskStatus]int64, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("stats: query: %w", err)
	}
	defer rows.Close()

	stats := make(map[domain.TaskStatus]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("stats: scan: %w", err)
		}
		stats[domain.TaskStatus(status)] = count
	}
	return stats, rows.Err()
}

// Heartbeat registers or updates a worker's last_seen_at timestamp.
func (q *PostgresQueue) Heartbeat(ctx context.Context, workerID string) error {
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	host, _ := os.Hostname()
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO workers (worker_id, hostname, last_seen_at, status)
		VALUES ($1, $2, clock_timestamp(), 'active')
		ON CONFLICT (worker_id) DO UPDATE
		SET last_seen_at = clock_timestamp(), hostname = $2, status = 'active'
	`, workerID, host)
	return err
}

// RenewLease extends a live lease only when the complete ownership credential
// (task, worker, fencing generation) still matches. An already expired lease
// cannot be resurrected, even if no replacement worker has claimed it yet.
func (q *PostgresQueue) RenewLease(ctx context.Context, taskID string, workerID string, leaseVersion int, leaseTimeout time.Duration) error {
	ctx, span := otel.Tracer("distributed-task-lab/postgres").Start(ctx, "lease.renew",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("task.id", taskID), attribute.String("worker_id", workerID), attribute.Int("lease_version", leaseVersion)))
	defer span.End()
	ctx, cancel := q.operationContext(ctx)
	defer cancel()
	res, err := q.db.ExecContext(ctx, `
		UPDATE tasks
		SET lease_expires_at = clock_timestamp() + ($1 * interval '1 microsecond'),
		    updated_at = clock_timestamp()
		WHERE id = $2
		  AND status = 'running'
		  AND leased_by = $3
		  AND lease_version = $4
		  AND lease_expires_at >= clock_timestamp()
	`, leaseTimeout.Microseconds(), taskID, workerID, leaseVersion)
	if err != nil {
		return fmt.Errorf("renew lease: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("renew lease: task %s: %w", taskID, domain.ErrTaskNotClaimed)
	}
	return nil
}

// DetectStaleWorkers returns worker IDs that haven't sent a heartbeat within the threshold.
func (q *PostgresQueue) DetectStaleWorkers(ctx context.Context, threshold time.Duration) ([]string, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT worker_id FROM workers
		WHERE last_seen_at < clock_timestamp() - ($1 * interval '1 microsecond')
		  AND status = 'active'
	`, threshold.Microseconds())
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

const latestSchemaVersion int64 = 9

func (q *PostgresQueue) HealthCheck(ctx context.Context) error {
	var version sql.NullInt64
	var count int64
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*), MAX(version) FROM schema_migrations`).Scan(&count, &version); err != nil {
		return fmt.Errorf("health: database/schema unavailable: %w", err)
	}
	if !version.Valid || version.Int64 != latestSchemaVersion || count != latestSchemaVersion {
		return fmt.Errorf("health: schema version/count %d/%d, want %d/%d", version.Int64, count, latestSchemaVersion, latestSchemaVersion)
	}
	return nil
}

func (q *PostgresQueue) DBStats() domain.DBPoolStats {
	s := q.db.Stats()
	return domain.DBPoolStats{OpenConnections: s.OpenConnections, InUse: s.InUse, Idle: s.Idle,
		WaitCount: s.WaitCount, WaitDurationSeconds: s.WaitDuration.Seconds()}
}

// --- Internal helpers ---

func scanTask(row *sql.Row) (*domain.Task, error) {
	var t domain.Task
	var payloadBytes, resultBytes []byte

	err := row.Scan(
		&t.ID, &t.Type, &t.Queue, &t.Status, &payloadBytes, &resultBytes,
		&t.Priority, &t.MaxAttempts, &t.Attempt, &t.TimeoutSeconds, &t.RetryDelaySeconds,
		&t.RetryMaxDelaySeconds, &t.RetryBackoffMultiplier, &t.RetryJitter,
		&t.LeasedBy, &t.LeaseExpiresAt, &t.LeaseVersion,
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
		&t.ID, &t.Type, &t.Queue, &t.Status, &payloadBytes, &resultBytes,
		&t.Priority, &t.MaxAttempts, &t.Attempt, &t.TimeoutSeconds, &t.RetryDelaySeconds,
		&t.RetryMaxDelaySeconds, &t.RetryBackoffMultiplier, &t.RetryJitter,
		&t.LeasedBy, &t.LeaseExpiresAt, &t.LeaseVersion,
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
