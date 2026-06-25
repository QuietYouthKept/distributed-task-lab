package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yourusername/distributed-task-lab/internal/domain"
)

// Client is a simple HTTP client for the task API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// CreateTask submits a new task.
func (c *Client) CreateTask(req CreateTaskRequest) (*CreateTaskResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("client: marshal: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("client: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("client: decode: %w", err)
	}
	return &result, nil
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(id string) (*domain.Task, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/tasks/" + id)
	if err != nil {
		return nil, fmt.Errorf("client: get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client: unexpected status %d", resp.StatusCode)
	}

	var task domain.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, fmt.Errorf("client: decode: %w", err)
	}
	return &task, nil
}

// ListTasks lists tasks with optional status filter.
func (c *Client) ListTasks(status *domain.TaskStatus) ([]domain.Task, error) {
	url := c.baseURL + "/tasks"
	if status != nil {
		url += "?status=" + string(*status)
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("client: list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client: unexpected status %d", resp.StatusCode)
	}

	var tasks []domain.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("client: decode: %w", err)
	}
	return tasks, nil
}

// CreateTaskRequest is the request body for creating a task.
type CreateTaskRequest struct {
	Type              domain.TaskType            `json:"type"`
	Payload           domain.TaskPayloadEnvelope `json:"payload"`
	Priority          int                        `json:"priority,omitempty"`
	MaxAttempts       int                        `json:"max_attempts,omitempty"`
	TimeoutSeconds    int                        `json:"timeout_seconds,omitempty"`
	RetryDelaySeconds int                        `json:"retry_delay_seconds,omitempty"`
	IdempotencyKey    *string                    `json:"idempotency_key,omitempty"`
}

// CreateTaskResponse is the response from creating a task.
type CreateTaskResponse struct {
	ID     string            `json:"id"`
	Status domain.TaskStatus `json:"status"`
}
