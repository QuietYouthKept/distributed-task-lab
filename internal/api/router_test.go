package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/api/handlers"
)

type readinessStub struct{ err error }

func (s readinessStub) HealthCheck(context.Context) error { return s.err }

func testRouter(ready ReadinessProvider, cfg RouterConfig) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouterWithConfig(&handlers.TaskHandler{}, &handlers.MetricsHandler{}, ready, logger, cfg)
}

func TestReadinessFailsClosed(t *testing.T) {
	h := testRouter(readinessStub{err: errors.New("db down")}, RouterConfig{})
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestAdminTokenBoundary(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{APIToken: "user", AdminToken: "admin"})
	for _, tc := range []struct {
		path  string
		token string
		want  int
	}{{path: "/queues/default/pause", token: "", want: 401}, {path: "/queues/default/pause", token: "user", want: 401}, {path: "/v1/queues/default/pause", token: "user", want: 401}} {
		r := httptest.NewRequest(http.MethodPost, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("token=%q status=%d want=%d", tc.token, w.Code, tc.want)
		}
	}
}

func TestV1RoutesUseSameStrictContract(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	r := httptest.NewRequest(http.MethodPost, "/v1/tasks", strings.NewReader(`{"unexpected":true}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"invalid_json"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSecurityHeadersOnLiveness(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	r := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
	}
}

func TestRequestIDIsGeneratedAndInvalidCallerIDIsReplaced(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	r := httptest.NewRequest(http.MethodGet, "/live", nil)
	r.Header.Set("X-Request-ID", "invalid value with spaces")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("X-Request-ID"); got == "" || got == "invalid value with spaces" {
		t.Fatalf("X-Request-ID=%q", got)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{MaxRequestBodyBytes: 32})
	body := strings.NewReader(`{"payload":"` + strings.Repeat("x", 100) + `"}`)
	r := httptest.NewRequest(http.MethodPost, "/tasks", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsUnknownTopLevelField(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	body := strings.NewReader(`{"type":"rag_ingestion","unexpected":true}`)
	r := httptest.NewRequest(http.MethodPost, "/tasks", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"invalid_json"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsMultipleJSONValues(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	body := strings.NewReader(`{} {}`)
	r := httptest.NewRequest(http.MethodPost, "/tasks", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"invalid_json"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsIdempotencyKeyMismatch(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	body := strings.NewReader(`{"idempotency_key":"body-key"}`)
	r := httptest.NewRequest(http.MethodPost, "/tasks", body)
	r.Header.Set("Idempotency-Key", "header-key")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"code":"idempotency_key_mismatch"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsDuplicateIdempotencyKeyHeaders(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	r := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{}`))
	r.Header.Add("Idempotency-Key", "key-one")
	r.Header.Add("Idempotency-Key", "key-two")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"code":"invalid_idempotency_key"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListTasksRejectsInvalidFilters(t *testing.T) {
	tests := []string{
		"/tasks?status=waiting",
		"/tasks?type=unknown",
		"/tasks?queue=%20default",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			h := testRouter(readinessStub{}, RouterConfig{})
			r := httptest.NewRequest(http.MethodGet, target, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"code":"invalid_filter"`) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMutationBodiesRejectUnknownFields(t *testing.T) {
	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/tasks/task-1/requeue"},
		{method: http.MethodPost, path: "/tasks/task-1/cancel"},
		{method: http.MethodPut, path: "/queues/default"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			h := testRouter(readinessStub{}, RouterConfig{})
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"unknown":true}`))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"invalid_json"`) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
