package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
	"github.com/google/uuid"
)

// TaskService orchestrates task lifecycle operations.
type TaskService struct {
	q queue.Queue
}

// NewTaskService creates a new task service backed by the given queue.
func NewTaskService(q queue.Queue) *TaskService {
	return &TaskService{q: q}
}

// CreateTaskRequest is the input for creating a new task.
type CreateTaskRequest struct {
	Type                   domain.TaskType            `json:"type"`
	Queue                  string                     `json:"queue"`
	Payload                domain.TaskPayloadEnvelope `json:"payload"`
	Priority               int                        `json:"priority"`
	MaxAttempts            int                        `json:"max_attempts"`
	TimeoutSeconds         int                        `json:"timeout_seconds"`
	RetryDelaySeconds      int                        `json:"retry_delay_seconds"`
	RetryMaxDelaySeconds   int                        `json:"retry_max_delay_seconds"`
	RetryBackoffMultiplier float64                    `json:"retry_backoff_multiplier"`
	RetryJitter            *bool                      `json:"retry_jitter"`
	IdempotencyKey         *string                    `json:"idempotency_key"`
}

// CreateTaskResponse is the result of creating a task.
type CreateTaskResponse struct {
	ID     string            `json:"id"`
	Status domain.TaskStatus `json:"status"`
}

// CreateTask creates a new task and enqueues it. If an idempotency_key is
// provided and a task with that key already exists, the existing task is
// returned instead of creating a duplicate.
func (s *TaskService) CreateTask(ctx context.Context, req CreateTaskRequest) (*CreateTaskResponse, error) {
	if !domain.ValidTaskTypes[req.Type] {
		return nil, domain.InvalidArgument("invalid_task_type", "task type is not supported", domain.ErrInvalidTaskType)
	}
	canonicalPayload, err := taskpayload.Validate(req.Type, req.Payload)
	if err != nil {
		return nil, domain.InvalidArgument("invalid_task_payload", "task payload does not match its task type", err)
	}
	if req.MaxAttempts < 0 || req.TimeoutSeconds < 0 || req.RetryDelaySeconds < 0 || req.RetryMaxDelaySeconds < 0 || req.RetryBackoffMultiplier < 0 {
		return nil, domain.InvalidArgument("invalid_retry_policy", "retry and timeout values must be non-negative", nil)
	}

	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 3
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 60
	}
	if req.RetryDelaySeconds <= 0 {
		req.RetryDelaySeconds = 5
	}
	if req.RetryBackoffMultiplier <= 0 {
		req.RetryBackoffMultiplier = 2.0
	}
	jitter := true
	if req.RetryJitter != nil {
		jitter = *req.RetryJitter
	}

	queue := req.Queue
	if queue == "" {
		queue = domain.DefaultQueueName
	}
	if len(queue) > 128 || strings.TrimSpace(queue) != queue {
		return nil, domain.InvalidArgument("invalid_queue", "queue name is invalid", nil)
	}

	task := &domain.Task{
		ID:                     uuid.New().String(),
		Type:                   req.Type,
		Queue:                  queue,
		Payload:                domain.TaskPayloadEnvelope{Version: req.Payload.Version, Kind: req.Payload.Kind, Data: canonicalPayload},
		Priority:               req.Priority,
		MaxAttempts:            req.MaxAttempts,
		TimeoutSeconds:         req.TimeoutSeconds,
		RetryDelaySeconds:      req.RetryDelaySeconds,
		RetryMaxDelaySeconds:   req.RetryMaxDelaySeconds,
		RetryBackoffMultiplier: req.RetryBackoffMultiplier,
		RetryJitter:            jitter,
		IdempotencyKey:         req.IdempotencyKey,
	}

	created, err := s.q.Enqueue(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("task service: enqueue: %w", err)
	}

	return &CreateTaskResponse{
		ID:     created.ID,
		Status: created.Status,
	}, nil
}

// GetTask retrieves a task by ID.
func (s *TaskService) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	task, err := s.q.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task service: get: %w", err)
	}
	if task == nil {
		return nil, domain.ErrTaskNotFound
	}
	return task, nil
}

// ListTasks lists tasks with optional filters.
func (s *TaskService) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	tasks, err := s.q.ListTasks(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("task service: list: %w", err)
	}
	if tasks == nil {
		tasks = []domain.Task{}
	}
	return tasks, nil
}

// ListDLQ lists tasks in the dead letter queue.
func (s *TaskService) ListDLQ(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	tasks, err := s.q.ListDLQ(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("task service: list dlq: %w", err)
	}
	if tasks == nil {
		tasks = []domain.Task{}
	}
	return tasks, nil
}

// Requeue resets a DLQ task back to pending.
func (s *TaskService) Requeue(ctx context.Context, taskID string, reason string) error {
	if err := s.q.Requeue(ctx, taskID, reason); err != nil {
		return fmt.Errorf("task service: requeue: %w", err)
	}
	return nil
}

// Cancel cancels a pending or running task.
func (s *TaskService) Cancel(ctx context.Context, taskID string, reason string) error {
	if err := s.q.Cancel(ctx, taskID, reason); err != nil {
		return fmt.Errorf("task service: cancel: %w", err)
	}
	return nil
}

// GetTaskEvents retrieves the audit event log for a task.
func (s *TaskService) GetTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error) {
	events, err := s.q.GetTaskEvents(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task service: events: %w", err)
	}
	return events, nil
}

// PauseQueue pauses a queue so workers stop claiming its tasks.
func (s *TaskService) PauseQueue(ctx context.Context, name string) error {
	if err := s.q.PauseQueue(ctx, name); err != nil {
		return fmt.Errorf("task service: pause queue: %w", err)
	}
	return nil
}

// ResumeQueue resumes a paused queue.
func (s *TaskService) ResumeQueue(ctx context.Context, name string) error {
	if err := s.q.ResumeQueue(ctx, name); err != nil {
		return fmt.Errorf("task service: resume queue: %w", err)
	}
	return nil
}

// ListQueues returns all queues with their governance config.
func (s *TaskService) ListQueues(ctx context.Context) ([]domain.QueueConfig, error) {
	queues, err := s.q.ListQueues(ctx)
	if err != nil {
		return nil, fmt.Errorf("task service: list queues: %w", err)
	}
	return queues, nil
}

// SetQueueConfig upserts a queue's max concurrency and rate limit.
func (s *TaskService) SetQueueConfig(ctx context.Context, config domain.QueueConfig) error {
	if config.Name == "" || len(config.Name) > 128 || strings.TrimSpace(config.Name) != config.Name {
		return domain.InvalidArgument("invalid_queue", "queue name is invalid", nil)
	}
	if config.MaxConcurrency < 0 || config.RateLimitPerSecond < 0 || config.Burst < 0 || config.AgingAfterSeconds < 0 || config.AgingPriorityBoost < 0 {
		return domain.InvalidArgument("invalid_queue_config", "queue limits must be non-negative", nil)
	}
	if config.Burst == 0 {
		config.Burst = 1
	}
	if err := s.q.SetQueueConfig(ctx, config); err != nil {
		return fmt.Errorf("task service: set queue config: %w", err)
	}
	return nil
}

func (s *TaskService) HealthCheck(ctx context.Context) error {
	return s.q.HealthCheck(ctx)
}

func (s *TaskService) GetDBPoolStats() domain.DBPoolStats { return s.q.DBStats() }

// GetStats returns the number of tasks currently in each status.
func (s *TaskService) GetStats(ctx context.Context) (map[domain.TaskStatus]int64, error) {
	stats, err := s.q.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("task service: stats: %w", err)
	}
	if stats == nil {
		stats = map[domain.TaskStatus]int64{}
	}
	return stats, nil
}

// GetQueueDepth returns task counts grouped by queue and status.
func (s *TaskService) GetQueueDepth(ctx context.Context) (map[string]map[domain.TaskStatus]int64, error) {
	depth, err := s.q.QueueDepth(ctx)
	if err != nil {
		return nil, fmt.Errorf("task service: queue depth: %w", err)
	}
	return depth, nil
}

// GetOldestPendingAge returns the age of the oldest pending task.
func (s *TaskService) GetOldestPendingAge(ctx context.Context) (time.Duration, error) {
	age, err := s.q.OldestPendingAge(ctx)
	if err != nil {
		return 0, fmt.Errorf("task service: oldest pending age: %w", err)
	}
	return age, nil
}
