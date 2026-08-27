package queue

import (
	"context"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

// Queue defines the abstract interface for task queue operations.
// Implementations must ensure ClaimTasks is atomic and that MarkSuccess/
// MarkFailure verify both worker ownership and the lease_version fencing token.
type Queue interface {
	// Enqueue inserts a new task and sets its initial state to pending. It is
	// idempotent on the task's idempotency_key: if a task with the same key
	// exists, that task is returned instead of inserting a duplicate.
	Enqueue(ctx context.Context, task *domain.Task) (*domain.Task, error)

	// GetTask retrieves a single task by its ID.
	GetTask(ctx context.Context, taskID string) (*domain.Task, error)

	// ListTasks retrieves tasks matching the given filter.
	ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error)

	// ClaimTasks atomically claims up to `limit` pending (or expired-lease) tasks
	// for the given workerID. The lease is set for leaseTimeout from now and the
	// lease_version fencing token is incremented.
	ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error)

	// MarkSuccess marks a task as successfully completed. Must verify the task is
	// currently leased by workerID and that leaseVersion matches the claim.
	MarkSuccess(ctx context.Context, taskID string, workerID string, leaseVersion int, result domain.TaskResult) error

	// MarkFailure marks a task as failed and applies retry/DLQ policy. Must verify
	// ownership and leaseVersion. Non-retryable failures go straight to the DLQ.
	MarkFailure(ctx context.Context, taskID string, workerID string, leaseVersion int, failure domain.TaskFailure) error

	// ListDLQ retrieves tasks currently in the dead letter queue.
	ListDLQ(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error)

	// Requeue resets a DLQ task back to pending (manual retry).
	Requeue(ctx context.Context, taskID string, reason string) error

	// Cancel marks a task as cancelled (pending or running).
	Cancel(ctx context.Context, taskID string, reason string) error

	// GetTaskEvents retrieves the audit log for a task.
	GetTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error)

	// Heartbeat registers or updates a worker's liveness.
	Heartbeat(ctx context.Context, workerID string) error

	// RenewLease extends the lease on a task currently owned by the worker.
	RenewLease(ctx context.Context, taskID string, workerID string, leaseVersion int, leaseTimeout time.Duration) error

	// HealthCheck verifies that PostgreSQL is reachable and that the embedded
	// schema version is installed. Implementations should fail closed.
	HealthCheck(ctx context.Context) error
	DBStats() domain.DBPoolStats

	// Stats returns the number of tasks currently in each status.
	Stats(ctx context.Context) (map[domain.TaskStatus]int64, error)

	// PauseQueue pauses a queue so workers stop claiming its tasks.
	PauseQueue(ctx context.Context, name string) error

	// ResumeQueue resumes a paused queue.
	ResumeQueue(ctx context.Context, name string) error

	// ListQueues returns all queues with their governance config.
	ListQueues(ctx context.Context) ([]domain.QueueConfig, error)

	// SetQueueConfig upserts a queue's max concurrency and rate limit.
	SetQueueConfig(ctx context.Context, config domain.QueueConfig) error

	// QueueDepth returns task counts grouped by queue and status.
	QueueDepth(ctx context.Context) (map[string]map[domain.TaskStatus]int64, error)

	// OldestPendingAge returns the age of the oldest pending task (0 if none).
	OldestPendingAge(ctx context.Context) (time.Duration, error)
}
