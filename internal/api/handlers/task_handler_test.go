package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/application"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
)

type enqueueErrorQueue struct {
	queue.Queue
	err error
}

type listTasksQueue struct {
	queue.Queue
	tasks []domain.Task
}

func (q listTasksQueue) ListTasks(_ context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	result := make([]domain.Task, 0, filter.Limit)
	for _, task := range q.tasks {
		if filter.AfterCreatedAt != nil && (task.CreatedAt.After(*filter.AfterCreatedAt) || (task.CreatedAt.Equal(*filter.AfterCreatedAt) && task.ID >= *filter.AfterID)) {
			continue
		}
		result = append(result, task)
		if filter.Limit > 0 && len(result) == filter.Limit {
			break
		}
	}
	return result, nil
}

func (q enqueueErrorQueue) Enqueue(context.Context, *domain.Task) (*domain.Task, error) {
	return nil, q.err
}

func TestCreateTaskMapsDatabaseTimeoutToSafe503(t *testing.T) {
	secretCause := fmt.Errorf("query SELECT secret with postgres://user:password@db token=abc: %w", context.DeadlineExceeded)
	svc := application.NewTaskService(enqueueErrorQueue{err: secretCause})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewTaskHandler(svc, logger)
	body := `{"type":"rag_ingestion","payload":{"version":"1","kind":"rag_ingestion","data":{"execution_mode":"mock","document_id":"doc-1"}}}`
	r := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	response := w.Body.String()
	if !strings.Contains(response, `"code":"database_unavailable"`) {
		t.Fatalf("missing stable error code: %s", response)
	}
	for _, leaked := range []string{"deadline exceeded", "task service: enqueue", "postgres://", "token="} {
		if strings.Contains(response, leaked) {
			t.Errorf("response leaked internal detail %q: %s", leaked, response)
		}
	}
}

func TestV1ListTasksReturnsSignedKeysetPages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	baseTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	tasks := []domain.Task{
		{ID: "task-1", CreatedAt: baseTime.Add(-1 * time.Second)},
		{ID: "task-2", CreatedAt: baseTime.Add(-2 * time.Second)},
		{ID: "task-3", CreatedAt: baseTime.Add(-3 * time.Second)},
		{ID: "task-4", CreatedAt: baseTime.Add(-4 * time.Second)},
	}
	h := NewTaskHandler(application.NewTaskService(listTasksQueue{tasks: tasks}), logger)
	firstReq := httptest.NewRequest(http.MethodGet, "/v1/tasks?page_size=2", nil)
	firstResponse := httptest.NewRecorder()
	h.ListTasks(firstResponse, firstReq)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first struct {
		Tasks []domain.Task `json:"tasks"`
		Next  string        `json:"next_cursor"`
		More  bool          `json:"has_more"`
	}
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Tasks) != 2 || first.Next == "" || !first.More {
		t.Fatalf("unexpected first page: %+v", first)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1/tasks?page_size=2&cursor="+url.QueryEscape(first.Next), nil)
	secondResponse := httptest.NewRecorder()
	h.ListTasks(secondResponse, secondReq)
	var second struct {
		Tasks []domain.Task `json:"tasks"`
		More  bool          `json:"has_more"`
	}
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if secondResponse.Code != http.StatusOK || len(second.Tasks) != 2 || second.More {
		t.Fatalf("unexpected second page: status=%d page=%+v", secondResponse.Code, second)
	}
	if second.Tasks[0].ID == first.Tasks[1].ID || second.Tasks[0].ID != "task-3" {
		t.Fatalf("keyset did not advance: first=%+v second=%+v", first.Tasks, second.Tasks)
	}

	mismatch := httptest.NewRequest(http.MethodGet, "/v1/tasks?page_size=1&cursor="+url.QueryEscape(first.Next), nil)
	mismatchResponse := httptest.NewRecorder()
	h.ListTasks(mismatchResponse, mismatch)
	if mismatchResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(mismatchResponse.Body.String(), "invalid_cursor") {
		t.Fatalf("page size mismatch accepted: %d %s", mismatchResponse.Code, mismatchResponse.Body.String())
	}
}

func TestApplyIdempotencyKeyFromHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	r.Header.Set("Idempotency-Key", "request-123")
	req := application.CreateTaskRequest{}

	if err := applyIdempotencyKey(r, &req); err != nil {
		t.Fatalf("applyIdempotencyKey: %v", err)
	}
	if req.IdempotencyKey == nil || *req.IdempotencyKey != "request-123" {
		t.Fatalf("idempotency key = %v, want request-123", req.IdempotencyKey)
	}
}
