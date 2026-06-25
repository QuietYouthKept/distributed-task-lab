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
}
