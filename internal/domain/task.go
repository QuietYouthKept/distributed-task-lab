package domain

import "time"

// Task is the core domain entity representing a unit of work.
type Task struct {
	ID     string
	Type   TaskType
	Status TaskStatus

	Payload TaskPayloadEnvelope
	Result  *TaskResult

	Priority          int
	MaxAttempts       int
	Attempt           int
	TimeoutSeconds    int
	RetryDelaySeconds int

	LeasedBy       *string
	LeaseExpiresAt *time.Time

	NextRunAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time

	LastError *string
	DLQReason *string

	IdempotencyKey *string
}

// TaskFilter is used to query tasks with optional filters.
type TaskFilter struct {
	Status *TaskStatus
	Type   *TaskType
	Limit  int
	Offset int
}

// RetryPolicy defines how a task should be retried.
type RetryPolicy struct {
	MaxAttempts       int
	RetryDelaySeconds int
}

// DefaultRetryPolicy returns the default retry configuration.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:       3,
		RetryDelaySeconds: 5,
	}
}
