package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxResponseBytes = 1 << 20

// Client is a context-aware client for the stable /v1 task API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	bearer     string
}

// APIError retains the stable error contract without exposing arbitrary body
// data to callers. Unwrap is intentionally absent: server internals are not
// represented in the public API.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("task API: HTTP %d %s: %s (request_id=%s)", e.StatusCode, e.Code, e.Message, e.RequestID)
}

// NewClient creates a client with the default transport and no credentials.
func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: &http.Client{}}
}

// WithHTTPClient returns the client with a caller-owned HTTP client. The
// supplied client should have an appropriate timeout for the application.
func (c *Client) WithHTTPClient(client *http.Client) *Client {
	if client != nil {
		c.httpClient = client
	}
	return c
}

// WithBearerToken configures the OIDC access token used on subsequent calls.
func (c *Client) WithBearerToken(token string) *Client {
	c.bearer = strings.TrimSpace(token)
	return c
}

// CreateTask preserves the original convenience API. Prefer CreateTaskContext
// when the caller needs cancellation or a deadline.
func (c *Client) CreateTask(req CreateTaskRequest) (*CreateTaskResponse, error) {
	return c.CreateTaskContext(context.Background(), req)
}

func (c *Client) CreateTaskContext(ctx context.Context, req CreateTaskRequest) (*CreateTaskResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("client: marshal: %w", err)
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/tasks", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.IdempotencyKey != nil {
		httpReq.Header.Set("Idempotency-Key", *req.IdempotencyKey)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("client: create task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, decodeAPIError(resp)
	}
	var result CreateTaskResponse
	if err := decodeBounded(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("client: decode create response: %w", err)
	}
	return &result, nil
}

func (c *Client) GetTask(id string) (*Task, error) {
	return c.GetTaskContext(context.Background(), id)
}

func (c *Client) GetTaskContext(ctx context.Context, id string) (*Task, error) {
	if id == "" {
		return nil, fmt.Errorf("client: task id is required")
	}
	path := "/tasks/" + url.PathEscape(id)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("client: get task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}
	var task Task
	if err := decodeBounded(resp.Body, &task); err != nil {
		return nil, fmt.Errorf("client: decode task: %w", err)
	}
	return &task, nil
}

func (c *Client) ListTasks(status *TaskStatus) ([]Task, error) {
	return c.ListTasksContext(context.Background(), status)
}

func (c *Client) ListTasksContext(ctx context.Context, status *TaskStatus) ([]Task, error) {
	page, err := c.ListTasksPageContext(ctx, TaskPageOptions{Status: status, PageSize: 100})
	if err != nil {
		return nil, err
	}
	return page.Tasks, nil
}

// ListTasksPageContext retrieves one keyset-paginated page. Pass NextCursor
// from the previous response to continue; cursors are bound to the principal,
// normalized filters, and page size by the server.
func (c *Client) ListTasksPageContext(ctx context.Context, options TaskPageOptions) (*TaskPage, error) {
	query := url.Values{}
	if options.Status != nil {
		query.Set("status", string(*options.Status))
	}
	if options.Type != nil {
		query.Set("type", string(*options.Type))
	}
	if options.Queue != "" {
		query.Set("queue", options.Queue)
	}
	if options.PageSize != 0 {
		query.Set("page_size", strconv.Itoa(options.PageSize))
	}
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	path := "/tasks"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("client: list tasks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}
	var page TaskPage
	if err := decodeBounded(resp.Body, &page); err != nil {
		return nil, fmt.Errorf("client: decode task page: %w", err)
	}
	return &page, nil
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if ctx == nil {
		return nil, fmt.Errorf("client: context is nil")
	}
	if c.baseURL == "" {
		return nil, fmt.Errorf("client: base URL is required")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("client: base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	route, err := url.Parse("/v1" + path)
	if err != nil {
		return nil, fmt.Errorf("client: build route: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + route.Path
	base.RawPath = ""
	base.RawQuery = route.RawQuery
	req, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return nil, fmt.Errorf("client: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	return req, nil
}

func decodeBounded(reader io.Reader, target any) error {
	raw, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
	}
	return json.Unmarshal(raw, target)
}

func decodeAPIError(resp *http.Response) error {
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := decodeBounded(resp.Body, &body); err != nil {
		return &APIError{StatusCode: resp.StatusCode, Code: "invalid_error_response", Message: "server returned an invalid error response", RequestID: resp.Header.Get("X-Request-ID")}
	}
	return &APIError{StatusCode: resp.StatusCode, Code: body.Error.Code, Message: body.Error.Message, RequestID: body.RequestID}
}

// CreateTaskRequest is the request body for creating a task.
type CreateTaskRequest struct {
	Type              TaskType            `json:"type"`
	Payload           TaskPayloadEnvelope `json:"payload"`
	Priority          int                 `json:"priority,omitempty"`
	MaxAttempts       int                 `json:"max_attempts,omitempty"`
	TimeoutSeconds    int                 `json:"timeout_seconds,omitempty"`
	RetryDelaySeconds int                 `json:"retry_delay_seconds,omitempty"`
	IdempotencyKey    *string             `json:"idempotency_key,omitempty"`
}

// CreateTaskResponse is the response from creating a task.
type CreateTaskResponse struct {
	ID     string     `json:"id"`
	Status TaskStatus `json:"status"`
}

type TaskPageOptions struct {
	Status   *TaskStatus
	Type     *TaskType
	Queue    string
	PageSize int
	Cursor   string
}

type TaskPage struct {
	Tasks      []Task `json:"tasks"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}
