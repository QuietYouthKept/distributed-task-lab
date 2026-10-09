package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestCreateTaskContextUsesV1AuthAndIdempotencyHeaders(t *testing.T) {
	const key = "request-unique-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/v1/tasks" || r.Method != http.MethodPost {
			t.Errorf("request target = %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != key {
			t.Errorf("Idempotency-Key = %q", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"task-1","status":"pending"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL + "/prefix").WithBearerToken("access-token")
	result, err := client.CreateTaskContext(context.Background(), CreateTaskRequest{
		Type:           domain.TaskTypeRAGIngestion,
		Payload:        domain.TaskPayloadEnvelope{Version: "v1", Kind: "rag_ingestion", Data: map[string]any{}},
		IdempotencyKey: ptr(key),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "task-1" || result.Status != domain.TaskStatusPending {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestClientReturnsTypedAPIErrorAndHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "from-header")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"not allowed"},"request_id":"req-123"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).GetTaskContext(context.Background(), "task-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Code != "forbidden" || apiErr.RequestID != "req-123" {
		t.Fatalf("expected typed API error, got %#v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewClient(server.URL).GetTaskContext(ctx, "task-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestClientRejectsUnsafeBaseURL(t *testing.T) {
	client := NewClient("https://user:password@example.test")
	_, err := client.ListTasksContext(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("expected unsafe base URL rejection, got %v", err)
	}
}

func TestListTasksPageEncodesFiltersAndCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tasks" || r.URL.Query().Get("queue") != "jobs & reports" || r.URL.Query().Get("cursor") != "signed.cursor" || r.URL.Query().Get("page_size") != "10" {
			t.Errorf("unexpected page request: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"tasks":[],"next_cursor":"next","has_more":true}`))
	}))
	defer server.Close()
	page, err := NewClient(server.URL).ListTasksPageContext(context.Background(), TaskPageOptions{Queue: "jobs & reports", PageSize: 10, Cursor: "signed.cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || page.NextCursor != "next" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func ptr(value string) *string { return &value }
