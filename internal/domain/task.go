package domain

import "time"

// Task is the core domain entity representing a unit of work.
type Task struct {
	ID     string
	Type   TaskType
	Status TaskStatus
	Queue  string

	Payload TaskPayloadEnvelope
	Result  *TaskResult

	Priority          int
	MaxAttempts       int
	Attempt           int
	TimeoutSeconds    int
	RetryDelaySeconds int // base (initial) retry delay in seconds

	// Retry backoff configuration.
	RetryMaxDelaySeconds   int     // cap on backoff delay (0 = no cap)
	RetryBackoffMultiplier float64 // exponential factor (default 2.0)
	RetryJitter            bool    // apply full jitter

	LeasedBy       *string
	LeaseExpiresAt *time.Time
	LeaseVersion   int // fencing token, incremented on each claim

	NextRunAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time

	LastError *string
	DLQReason *string

	IdempotencyKey *string
}

// RetryDelayForAttempt computes the delay before the next retry for a task that
// just failed on the given 1-based attempt.
func (t Task) RetryDelayForAttempt(attempt int) time.Duration {
	base := time.Duration(t.RetryDelaySeconds) * time.Second
	max := time.Duration(t.RetryMaxDelaySeconds) * time.Second
	d := ExponentialBackoff(base, max, t.RetryBackoffMultiplier, attempt)
	if t.RetryJitter {
		d = FullJitter(d)
	}
	return d
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
