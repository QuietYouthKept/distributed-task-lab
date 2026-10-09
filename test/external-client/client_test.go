package externalclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	taskclient "github.com/fantasticyi/distributed-task-lab/pkg/client"
)

// This consumer lives in a separate Go module and intentionally imports only
// the public client package. It catches accidental dependencies on internal/.
func TestPublicClientFromIndependentModule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer external-token" {
			t.Errorf("authorization = %q", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/tasks":
			if got := r.Header.Get("Idempotency-Key"); got != "external-request-1" {
				t.Errorf("idempotency key = %q", got)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"task-external","status":"pending"}`))
		case "GET /v1/tasks/task-external":
			_ = json.NewEncoder(w).Encode(taskclient.Task{ID: "task-external", Type: taskclient.TaskTypeEval, Status: taskclient.TaskStatusSuccess})
		case "GET /v1/tasks":
			if r.URL.Query().Get("page_size") != "1" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"tasks":[{"id":"task-external","type":"eval","status":"success"}],"next_cursor":"cursor-1","has_more":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := taskclient.NewClient(server.URL).WithBearerToken("external-token")
	key := "external-request-1"
	created, err := client.CreateTaskContext(context.Background(), taskclient.CreateTaskRequest{
		Type: taskclient.TaskTypeEval,
		Payload: taskclient.TaskPayloadEnvelope{
			Version: "1", Kind: "eval", Data: map[string]any{"execution_mode": "mock"},
		},
		IdempotencyKey: &key,
	})
	if err != nil || created.ID != "task-external" {
		t.Fatalf("create: response=%+v err=%v", created, err)
	}

	got, err := client.GetTaskContext(context.Background(), created.ID)
	if err != nil || got.Status != taskclient.TaskStatusSuccess {
		t.Fatalf("get: task=%+v err=%v", got, err)
	}

	page, err := client.ListTasksPageContext(context.Background(), taskclient.TaskPageOptions{PageSize: 1})
	if err != nil || !page.HasMore || page.NextCursor != "cursor-1" || len(page.Tasks) != 1 {
		t.Fatalf("list: page=%+v err=%v", page, err)
	}
}
