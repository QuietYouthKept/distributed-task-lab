package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/application"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
)

type enqueueErrorQueue struct {
	queue.Queue
	err error
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
