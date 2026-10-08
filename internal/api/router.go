package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/api/handlers"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type ReadinessProvider interface{ HealthCheck(context.Context) error }

type RouterConfig struct {
	APIToken            string
	AdminToken          string
	MaxRequestBodyBytes int64
	RequestTimeout      time.Duration
}

func (c RouterConfig) withDefaults() RouterConfig {
	if c.MaxRequestBodyBytes <= 0 {
		c.MaxRequestBodyBytes = 1 << 20
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 10 * time.Second
	}
	return c
}

// NewRouter retains the local-development API. Production entrypoints should
// use NewRouterWithConfig to provide readiness and authentication settings.
func NewRouter(taskHandler *handlers.TaskHandler, metricsHandler *handlers.MetricsHandler, logger *slog.Logger) http.Handler {
	return NewRouterWithConfig(taskHandler, metricsHandler, nil, logger, RouterConfig{})
}

func NewRouterWithConfig(taskHandler *handlers.TaskHandler, metricsHandler *handlers.MetricsHandler, ready ReadinessProvider, logger *slog.Logger, cfg RouterConfig) http.Handler {
	cfg = cfg.withDefaults()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", taskHandler.CreateTask)
	mux.HandleFunc("GET /tasks", taskHandler.ListTasks)
	mux.HandleFunc("GET /tasks/{id}", taskHandler.GetTask)
	mux.HandleFunc("GET /dlq", taskHandler.ListDLQ)
	mux.HandleFunc("GET /stats", taskHandler.GetStats)
	mux.HandleFunc("GET /metrics", metricsHandler.ServeMetrics)
	mux.HandleFunc("POST /tasks/{id}/requeue", taskHandler.Requeue)
	mux.HandleFunc("POST /tasks/{id}/cancel", taskHandler.Cancel)
	mux.HandleFunc("GET /tasks/{id}/events", taskHandler.GetTaskEvents)
	mux.HandleFunc("GET /queues", taskHandler.ListQueues)
	mux.HandleFunc("PUT /queues/{name}", taskHandler.SetQueueConfig)
	mux.HandleFunc("POST /queues/{name}/pause", taskHandler.PauseQueue)
	mux.HandleFunc("POST /queues/{name}/resume", taskHandler.ResumeQueue)

	// /v1 is the stable public contract. The unversioned routes above remain as
	// a compatibility window for existing local scripts and demos.
	mux.HandleFunc("POST /v1/tasks", taskHandler.CreateTask)
	mux.HandleFunc("GET /v1/tasks", taskHandler.ListTasks)
	mux.HandleFunc("GET /v1/tasks/{id}", taskHandler.GetTask)
	mux.HandleFunc("GET /v1/dlq", taskHandler.ListDLQ)
	mux.HandleFunc("GET /v1/stats", taskHandler.GetStats)
	mux.HandleFunc("GET /v1/metrics", metricsHandler.ServeMetrics)
	mux.HandleFunc("POST /v1/tasks/{id}/requeue", taskHandler.Requeue)
	mux.HandleFunc("POST /v1/tasks/{id}/cancel", taskHandler.Cancel)
	mux.HandleFunc("GET /v1/tasks/{id}/events", taskHandler.GetTaskEvents)
	mux.HandleFunc("GET /v1/queues", taskHandler.ListQueues)
	mux.HandleFunc("PUT /v1/queues/{name}", taskHandler.SetQueueConfig)
	mux.HandleFunc("POST /v1/queues/{name}/pause", taskHandler.PauseQueue)
	mux.HandleFunc("POST /v1/queues/{name}/resume", taskHandler.ResumeQueue)

	live := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
	}
	mux.HandleFunc("GET /live", live)
	mux.HandleFunc("GET /health", live) // backwards-compatible liveness alias
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if ready == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.RequestTimeout)
		defer cancel()
		if err := ready.HealthCheck(ctx); err != nil {
			logger.Warn("readiness failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	var h http.Handler = mux
	h = authMiddleware(h, cfg)
	h = requestSafetyMiddleware(h, cfg)
	h = loggingMiddleware(h, logger)
	h = requestIDMiddleware(h)
	h = otelhttp.NewHandler(h, "task-lab.http")
	return h
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := contract.NormalizeRequestID(r.Header.Get("X-Request-ID"))
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(contract.WithRequestID(r.Context(), requestID)))
	})
}

func requestSafetyMiddleware(next http.Handler, cfg RouterConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxRequestBodyBytes)
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.RequestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authMiddleware(next http.Handler, cfg RouterConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/live" || r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}
		required := cfg.APIToken
		if isAdminMutation(r) && cfg.AdminToken != "" {
			required = cfg.AdminToken
		}
		if required != "" && !validBearer(r.Header.Get("Authorization"), required) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			contract.WriteError(w, r, domain.NewError(domain.ErrorKindUnauthorized, "unauthorized", "authentication is required", nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAdminMutation(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	return strings.HasPrefix(path, "/queues/") || strings.HasSuffix(path, "/requeue") || strings.HasSuffix(path, "/cancel")
}

func validBearer(header, token string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return len(got) == len(token) && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
