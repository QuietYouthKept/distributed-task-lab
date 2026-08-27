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
		token string
		want  int
	}{{"", 401}, {"user", 401}} {
		r := httptest.NewRequest(http.MethodPost, "/queues/default/pause", nil)
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

func TestSecurityHeadersOnLiveness(t *testing.T) {
	h := testRouter(readinessStub{}, RouterConfig{})
	r := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
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
