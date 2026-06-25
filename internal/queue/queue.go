package queue

import (
	"context"
	"time"

	"github.com/yourusername/distributed-task-lab/internal/domain"
)

// Queue defines the abstract interface for task queue operations.
// Implementations must ensure ClaimTasks is atomic and
// MarkSuccess/MarkFailure validate worker ownership.
type Queue interface {
	// Enqueue inserts a new task and sets its initial state to pending.
	Enqueue(ctx context.Context, task *domain.Task) error

	// GetTask retrieves a single task by its ID.
	GetTask(ctx context.Context, taskID string) (*domain.Task, error)

	// ListTasks retrieves tasks matching the given filter.
	ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error)

	// ClaimTasks atomically claims up to `limit` pending (or expired-lease) tasks
	// for the given workerID. The lease is set for leaseTimeout from now.
	ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error)

	// MarkSuccess marks a task as successfully completed.
	// Must verify that the task is currently leased by workerID.
	MarkSuccess(ctx context.Context, taskID string, workerID string, result domain.TaskResult) error

	// MarkFailure marks a task as failed and handles retry/DLQ logic.
	// Must verify that the task is currently leased by workerID.
	MarkFailure(ctx context.Context, taskID string, workerID string, failure domain.TaskFailure) error

	// ListDLQ retrieves tasks currently in the dead letter queue.
	ListDLQ(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error)

	// Requeue resets a DLQ task back to pending (manual retry).
	// Only allowed for tasks in 'dlq' status.
	Requeue(ctx context.Context, taskID string, reason string) error

	// Cancel marks a task as cancelled. Pending tasks are cancelled immediately;
	// running tasks require cooperative cancellation (best-effort).
	Cancel(ctx context.Context, taskID string, reason string) error

	// GetTaskEvents retrieves the audit log for a task.
	GetTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error)

	// Heartbeat registers or updates a worker's liveness.
	Heartbeat(ctx context.Context, workerID string) error

	// RenewLease extends the lease on a task currently owned by the worker.
	RenewLease(ctx context.Context, taskID string, workerID string, leaseTimeout time.Duration) error
}
