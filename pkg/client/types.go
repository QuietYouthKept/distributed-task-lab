package client

import "time"

// TaskType and TaskStatus are the stable public API values. They intentionally
// do not expose the service's internal domain package to external modules.
type TaskType string
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusDLQ     TaskStatus = "dlq"

	TaskTypeRAGIngestion   TaskType = "rag_ingestion"
	TaskTypeEval           TaskType = "eval"
	TaskTypeBenchmark      TaskType = "benchmark"
	TaskTypeStrategyReplay TaskType = "strategy_replay"
	TaskTypeNotification   TaskType = "notification"
	TaskTypePayment        TaskType = "payment"
)

// TaskPayloadEnvelope is the versioned task payload accepted by API v1.
type TaskPayloadEnvelope struct {
	Version string         `json:"version"`
	Kind    string         `json:"kind"`
	Data    map[string]any `json:"data"`
}

// TaskResult is the public task result representation.
type TaskResult struct {
	Summary   string             `json:"summary"`
	Artifacts map[string]string  `json:"artifacts,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

// Task is the stable API v1 task representation returned by GetTask and pages.
type Task struct {
	ID                     string              `json:"id"`
	Type                   TaskType            `json:"type"`
	Status                 TaskStatus          `json:"status"`
	Queue                  string              `json:"queue"`
	Payload                TaskPayloadEnvelope `json:"payload"`
	Result                 *TaskResult         `json:"result,omitempty"`
	Priority               int                 `json:"priority"`
	MaxAttempts            int                 `json:"max_attempts"`
	Attempt                int                 `json:"attempt"`
	TimeoutSeconds         int                 `json:"timeout_seconds"`
	RetryDelaySeconds      int                 `json:"retry_delay_seconds"`
	RetryMaxDelaySeconds   int                 `json:"retry_max_delay_seconds"`
	RetryBackoffMultiplier float64             `json:"retry_backoff_multiplier"`
	RetryJitter            bool                `json:"retry_jitter"`
	LeasedBy               *string             `json:"leased_by,omitempty"`
	LeaseExpiresAt         *time.Time          `json:"lease_expires_at,omitempty"`
	LeaseVersion           int                 `json:"lease_version"`
	NextRunAt              time.Time           `json:"next_run_at"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
	StartedAt              *time.Time          `json:"started_at,omitempty"`
	FinishedAt             *time.Time          `json:"finished_at,omitempty"`
	LastError              *string             `json:"last_error,omitempty"`
	DLQReason              *string             `json:"dlq_reason,omitempty"`
	IdempotencyKey         *string             `json:"idempotency_key,omitempty"`
	CreatedBy              *string             `json:"created_by,omitempty"`
}
