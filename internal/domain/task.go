package domain

import "time"

// Task is the core domain entity representing a unit of work.
// JSON tags are snake_case to match the documented API contract.
type Task struct {
	ID     string     `json:"id"`
	Type   TaskType   `json:"type"`
	Status TaskStatus `json:"status"`
	Queue  string     `json:"queue"`

	Payload TaskPayloadEnvelope `json:"payload"`
	Result  *TaskResult         `json:"result,omitempty"`

	Priority          int `json:"priority"`
	MaxAttempts       int `json:"max_attempts"`
	Attempt           int `json:"attempt"`
	TimeoutSeconds    int `json:"timeout_seconds"`
	RetryDelaySeconds int `json:"retry_delay_seconds"` // base (initial) retry delay in seconds

	// Retry backoff configuration.
	RetryMaxDelaySeconds   int     `json:"retry_max_delay_seconds"`
	RetryBackoffMultiplier float64 `json:"retry_backoff_multiplier"`
	RetryJitter            bool    `json:"retry_jitter"`

	LeasedBy       *string    `json:"leased_by,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	LeaseVersion   int        `json:"lease_version"` // fencing token, incremented on each claim

	NextRunAt  time.Time  `json:"next_run_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	LastError *string `json:"last_error,omitempty"`
	DLQReason *string `json:"dlq_reason,omitempty"`

	IdempotencyKey *string `json:"idempotency_key,omitempty"`
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
