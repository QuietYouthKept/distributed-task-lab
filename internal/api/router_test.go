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
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/handlers"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/observability"
)

type readinessStub struct{ err error }

func (s readinessStub) HealthCheck(context.Context) error { return s.err }

type verifierStub struct{ principal auth.Principal }

func (s verifierStub) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token != "signed-test-token" {
		return auth.Principal{}, errors.New("invalid token")
	}
	return s.principal, nil
}

func TestAPIResponseMetricsUseFiniteStatusClassOnly(t *testing.T) {
	var metrics observability.RuntimeMetrics
	h := apiResponseMetricsMiddleware(&metrics, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failed", http.StatusServiceUnavailable)
	}))
	r := httptest.NewRequest(http.MethodGet, "/v1/tasks/private-customer-id?token=secret", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", w.Code, http.StatusServiceUnavailable)
	}
	var out strings.Builder
	metrics.WritePrometheus(&out)
	if !strings.Contains(out.String(), `tasklab_api_requests_total{status_class="5xx"} 1`) {
		t.Fatalf("missing 5xx count: %s", out.String())
	}
	for _, secret := range []string{"private-customer-id", "token=secret"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("metric leaked request value %q", secret)
		}
	}
}

type sharedAdmissionStub struct {
	allowed bool
	retry   int
	err     error
}

func (s sharedAdmissionStub) Allow(context.Context, string) (bool, int, error) {
	return s.allowed, s.retry, s.err
}

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

func TestEffectReconciliationAlwaysRequiresAdminBoundary(t *testing.T) {
	for _, tc := range []struct {
		name                string
		cfg                 RouterConfig
		path, method, token string
	}{
		{"no credentials", RouterConfig{}, "/v1/effects", http.MethodGet, ""},
		{"API token cannot resolve", RouterConfig{APIToken: "reader-token"}, "/v1/effects/effect-1/resolve", http.MethodPost, "reader-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testRouter(readinessStub{}, tc.cfg)
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"reason":"reviewed"}`))
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401 body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestOIDCRouteAuthorizationRequiresExplicitRole(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal auth.Principal
		path      string
		method    string
		want      int
	}{
		{"reader cannot create", auth.Principal{Subject: "reader-1", Roles: map[auth.Role]struct{}{auth.RoleReader: {}}}, "/v1/tasks", http.MethodPost, http.StatusForbidden},
		{"operator cannot configure queue", auth.Principal{Subject: "operator-1", Roles: map[auth.Role]struct{}{auth.RoleOperator: {}}}, "/v1/queues/default/pause", http.MethodPost, http.StatusForbidden},
		{"reader cannot inspect effects", auth.Principal{Subject: "reader-1", Roles: map[auth.Role]struct{}{auth.RoleReader: {}}}, "/v1/effects", http.MethodGet, http.StatusForbidden},
		{"operator cannot resolve effects", auth.Principal{Subject: "operator-1", Roles: map[auth.Role]struct{}{auth.RoleOperator: {}}}, "/v1/effects/effect-1/resolve", http.MethodPost, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testRouter(readinessStub{}, RouterConfig{Authenticator: verifierStub{principal: tc.principal}, RequireAuthentication: true})
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			r.Header.Set("Authorization", "Bearer signed-test-token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestIngressRateLimitIsBoundToVerifiedPrincipal(t *testing.T) {
	principal := auth.Principal{Subject: "tenant-user-1", Roles: map[auth.Role]struct{}{auth.RoleReader: {}}}
	cfg := RouterConfig{Authenticator: verifierStub{principal: principal}, RequireAuthentication: true, IngressRatePerSecond: 0.01, IngressBurst: 1}
	h := testRouter(readinessStub{}, cfg)
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/v1/not-a-route", nil)
		r.Header.Set("Authorization", "Bearer signed-test-token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := request().Code; got != http.StatusNotFound {
		t.Fatalf("first request status=%d want 404", got)
	}
	limited := request()
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" || !strings.Contains(limited.Body.String(), `"request_id"`) {
		t.Fatalf("second request should be a correlated 429, got status=%d headers=%v body=%s", limited.Code, limited.Header(), limited.Body.String())
	}

	other := testRouter(readinessStub{}, RouterConfig{Authenticator: verifierStub{principal: auth.Principal{Subject: "tenant-user-2", Roles: map[auth.Role]struct{}{auth.RoleReader: {}}}}, RequireAuthentication: true, IngressRatePerSecond: 0.01, IngressBurst: 1})
	r := httptest.NewRequest(http.MethodGet, "/v1/not-a-route", nil)
	r.Header.Set("Authorization", "Bearer signed-test-token")
	w := httptest.NewRecorder()
	other.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("different principal should have an independent bucket; status=%d", w.Code)
	}
}

func TestIngressLimiterReclaimsOnlyFullyRefilledIdleBuckets(t *testing.T) {
	limiter := newIngressLimiter(1, 1)
	limiter.maxBuckets = 1
	start := time.Unix(100, 0)
	if allowed, _ := limiter.allow("idle-principal", start); !allowed {
		t.Fatal("first request should consume the initial token")
	}

	if allowed, _ := limiter.allow("new-principal", start.Add(time.Second-time.Nanosecond)); allowed {
		t.Fatal("a bucket must not be evicted before its token has fully refilled")
	}
	if len(limiter.buckets) != 1 {
		t.Fatalf("bucket count after premature request = %d, want 1", len(limiter.buckets))
	}

	if allowed, retryAfter := limiter.allow("new-principal", start.Add(time.Second)); !allowed {
		t.Fatalf("fully refilled idle bucket should be reclaimable; retry_after=%d", retryAfter)
	}
	if len(limiter.buckets) != 1 {
		t.Fatalf("bucket count after reclamation = %d, want 1", len(limiter.buckets))
	}
}

func TestSharedIngressFailureModesAndRetryAfter(t *testing.T) {
	principal := auth.Principal{Subject: "subject-1", Roles: map[auth.Role]struct{}{auth.RoleReader: {}}}
	request := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil).WithContext(auth.WithPrincipal(context.Background(), principal))
	nextCalls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	closed := newSharedIngressLimiter(sharedAdmissionStub{err: errors.New("database password=must-not-leak")}, false, logger).middleware(next)
	closedResponse := httptest.NewRecorder()
	closed.ServeHTTP(closedResponse, request)
	if closedResponse.Code != http.StatusServiceUnavailable || strings.Contains(closedResponse.Body.String(), "must-not-leak") || nextCalls != 0 {
		t.Fatalf("fail-closed response=%d body=%s next calls=%d", closedResponse.Code, closedResponse.Body.String(), nextCalls)
	}

	open := newSharedIngressLimiter(sharedAdmissionStub{err: errors.New("database unavailable")}, true, logger).middleware(next)
	openResponse := httptest.NewRecorder()
	open.ServeHTTP(openResponse, request)
	if openResponse.Code != http.StatusNoContent || nextCalls != 1 {
		t.Fatalf("fail-open response=%d next calls=%d", openResponse.Code, nextCalls)
	}

	denied := newSharedIngressLimiter(sharedAdmissionStub{allowed: false, retry: 7}, false, logger).middleware(next)
	deniedResponse := httptest.NewRecorder()
	denied.ServeHTTP(deniedResponse, request)
	if deniedResponse.Code != http.StatusTooManyRequests || deniedResponse.Header().Get("Retry-After") != "7" || nextCalls != 1 {
		t.Fatalf("denial response=%d retry-after=%q next calls=%d", deniedResponse.Code, deniedResponse.Header().Get("Retry-After"), nextCalls)
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
