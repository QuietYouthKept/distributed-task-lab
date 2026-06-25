package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/distributed-task-lab/internal/domain"
	"github.com/yourusername/distributed-task-lab/internal/queue"
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
	Type              domain.TaskType
	Payload           domain.TaskPayloadEnvelope
	Priority          int
	MaxAttempts       int
	TimeoutSeconds    int
	RetryDelaySeconds int
	IdempotencyKey    *string
}

// CreateTaskResponse is the result of creating a task.
type CreateTaskResponse struct {
	ID     string            `json:"id"`
	Status domain.TaskStatus `json:"status"`
}

// CreateTask creates a new task and enqueues it.
func (s *TaskService) CreateTask(ctx context.Context, req CreateTaskRequest) (*CreateTaskResponse, error) {
	if !domain.ValidTaskTypes[req.Type] {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidTaskType, req.Type)
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

	task := &domain.Task{
		ID:                uuid.New().String(),
		Type:              req.Type,
		Payload:           req.Payload,
		Priority:          req.Priority,
		MaxAttempts:       req.MaxAttempts,
		TimeoutSeconds:    req.TimeoutSeconds,
		RetryDelaySeconds: req.RetryDelaySeconds,
		NextRunAt:         time.Now().UTC(),
		IdempotencyKey:    req.IdempotencyKey,
	}

	if err := s.q.Enqueue(ctx, task); err != nil {
		return nil, fmt.Errorf("task service: enqueue: %w", err)
	}

	return &CreateTaskResponse{
		ID:     task.ID,
		Status: domain.TaskStatusPending,
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
