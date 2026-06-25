package domain

import "time"

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusDLQ     TaskStatus = "dlq"
)

// ValidTaskStatuses is the set of all valid task statuses.
var ValidTaskStatuses = map[TaskStatus]bool{
	TaskStatusPending: true,
	TaskStatusRunning: true,
	TaskStatusSuccess: true,
	TaskStatusFailed:  true,
	TaskStatusDLQ:     true,
}

// TaskType categorizes the kind of work a task represents.
type TaskType string

const (
	TaskTypeRAGIngestion TaskType = "rag_ingestion"
	TaskTypeEval         TaskType = "eval"
	TaskTypeBenchmark    TaskType = "benchmark"
)

// ValidTaskTypes is the set of all valid task types.
var ValidTaskTypes = map[TaskType]bool{
	TaskTypeRAGIngestion: true,
	TaskTypeEval:         true,
	TaskTypeBenchmark:    true,
}

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

// TaskPayloadEnvelope wraps task-type-specific payload data.
type TaskPayloadEnvelope struct {
	Version string         `json:"version"`
	Kind    string         `json:"kind"`
	Data    map[string]any `json:"data"`
}

// TaskResult holds the outcome of a successfully executed task.
type TaskResult struct {
	Summary   string            `json:"summary"`
	Artifacts map[string]string `json:"artifacts,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

// TaskFailure describes why a task execution failed.
type TaskFailure struct {
	ErrorType  string    `json:"error_type"`
	Message    string    `json:"message"`
	Retryable  bool      `json:"retryable"`
	OccurredAt time.Time `json:"occurred_at"`
}

// TaskFilter is used to query tasks with optional filters.
type TaskFilter struct {
	Status   *TaskStatus
	Type     *TaskType
	Limit    int
	Offset   int
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
