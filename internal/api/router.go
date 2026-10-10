package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/api/handlers"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type ReadinessProvider interface{ HealthCheck(context.Context) error }

type RouterConfig struct {
	APIToken              string
	AdminToken            string
	Authenticator         auth.TokenVerifier
	RequireAuthentication bool
	MaxRequestBodyBytes   int64
	RequestTimeout        time.Duration
	IngressRatePerSecond  float64
	IngressBurst          int
	SharedIngressLimiter  SharedIngressAdmission
	SharedIngressFailOpen bool
	EffectHandler         *handlers.EffectHandler
}

// SharedIngressAdmission is an optional cross-process admission candidate.
// It remains absent unless a deployment explicitly enables and wires it.
type SharedIngressAdmission interface {
	Allow(context.Context, string) (allowed bool, retryAfterSeconds int, err error)
}

func (c RouterConfig) withDefaults() RouterConfig {
	if c.MaxRequestBodyBytes <= 0 {
		c.MaxRequestBodyBytes = 1 << 20
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 10 * time.Second
	}
	if c.IngressRatePerSecond <= 0 {
		c.IngressRatePerSecond = 100
	}
	if c.IngressBurst <= 0 {
		c.IngressBurst = 200
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
	if cfg.EffectHandler != nil {
		mux.HandleFunc("GET /v1/effects", cfg.EffectHandler.List)
		mux.HandleFunc("POST /v1/effects/{id}/resolve", cfg.EffectHandler.Resolve)
	}

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
	h = newIngressLimiter(cfg.IngressRatePerSecond, cfg.IngressBurst).middleware(h)
	if cfg.SharedIngressLimiter != nil {
		h = newSharedIngressLimiter(cfg.SharedIngressLimiter, cfg.SharedIngressFailOpen, logger).middleware(h)
	}
	h = authMiddleware(h, cfg)
	h = requestSafetyMiddleware(h, cfg)
	h = loggingMiddleware(h, logger)
	h = requestIDMiddleware(h)
	h = otelhttp.NewHandler(h, "task-lab.http")
	return h
}

type sharedIngressLimiter struct {
	store    SharedIngressAdmission
	failOpen bool
	logger   *slog.Logger
}

func newSharedIngressLimiter(store SharedIngressAdmission, failOpen bool, logger *slog.Logger) *sharedIngressLimiter {
	return &sharedIngressLimiter{store: store, failOpen: failOpen, logger: logger}
}

func (l *sharedIngressLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal.Subject == "" {
			next.ServeHTTP(w, r)
			return
		}
		allowed, retryAfter, err := l.store.Allow(r.Context(), principal.Subject)
		if err != nil {
			if l.failOpen {
				if l.logger != nil {
					l.logger.Warn("shared API admission unavailable; retaining local overload protection")
				}
				next.ServeHTTP(w, r)
				return
			}
			contract.WriteError(w, r, domain.NewError(domain.ErrorKindUnavailable,
				"shared_rate_limit_unavailable", "shared request admission is unavailable", nil))
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, retryAfter)))
			contract.WriteRateLimitError(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ingressBucket struct {
	tokens  float64
	updated time.Time
}

type ingressLimiter struct {
	mu         sync.Mutex
	rate       float64
	burst      float64
	maxBuckets int
	buckets    map[string]ingressBucket
}

func newIngressLimiter(rate float64, burst int) *ingressLimiter {
	return &ingressLimiter{rate: rate, burst: float64(burst), maxBuckets: 10000, buckets: make(map[string]ingressBucket)}
}

func (l *ingressLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok || principal.Subject == "" {
			next.ServeHTTP(w, r)
			return
		}
		allowed, retryAfter := l.allow(principal.Subject, time.Now())
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			contract.WriteRateLimitError(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *ingressLimiter) allow(subject string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, exists := l.buckets[subject]
	if !exists {
		// Bound memory even when an issuer has a large or compromised set of
		// authenticated subjects. Reclaim only buckets whose tokens would
		// already have fully refilled, so an idle principal does not receive
		// more burst capacity than the normal token-bucket policy allows.
		if len(l.buckets) >= l.maxBuckets {
			for key, inactive := range l.buckets {
				elapsed := now.Sub(inactive.updated).Seconds()
				if elapsed > 0 && inactive.tokens+elapsed*l.rate >= l.burst {
					delete(l.buckets, key)
				}
			}
		}
		if len(l.buckets) >= l.maxBuckets {
			return false, 1
		}
		bucket = ingressBucket{tokens: l.burst, updated: now}
	} else {
		elapsed := now.Sub(bucket.updated).Seconds()
		if elapsed > 0 {
			bucket.tokens = math.Min(l.burst, bucket.tokens+elapsed*l.rate)
		}
		bucket.updated = now
	}
	if bucket.tokens < 1 {
		l.buckets[subject] = bucket
		return false, max(1, int(math.Ceil((1-bucket.tokens)/l.rate)))
	}
	bucket.tokens--
	l.buckets[subject] = bucket
	return true, 0
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
		if cfg.Authenticator != nil {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(w, r)
				return
			}
			principal, err := cfg.Authenticator.Verify(r.Context(), raw)
			if err != nil {
				writeUnauthorized(w, r)
				return
			}
			if !principal.HasRole(requiredRole(r)) {
				contract.WriteError(w, r, domain.NewError(domain.ErrorKindForbidden, "forbidden", "the authenticated principal is not allowed to perform this operation", nil))
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
			return
		}
		if cfg.RequireAuthentication || isSensitiveEndpoint(r) {
			writeUnauthorized(w, r)
			return
		}
		required := cfg.APIToken
		if isAdminMutation(r) {
			if isSensitiveEndpoint(r) {
				required = cfg.AdminToken
				if required == "" {
					writeUnauthorized(w, r)
					return
				}
			} else if cfg.AdminToken != "" {
				required = cfg.AdminToken
			}
		}
		if required != "" && !validBearer(r.Header.Get("Authorization"), required) {
			writeUnauthorized(w, r)
			return
		}
		if required != "" {
			role := auth.RoleReader
			subject := "legacy-api-token"
			if isAdminMutation(r) {
				role, subject = auth.RoleAdmin, "legacy-admin-token"
			}
			r = r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{Subject: subject, Kind: auth.KindService, Roles: map[auth.Role]struct{}{role: {}}}))
		}
		next.ServeHTTP(w, r)
	})
}

func requiredRole(r *http.Request) auth.Role {
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	if strings.HasPrefix(path, "/effects") && r.Method != http.MethodGet {
		return auth.RoleAdmin
	}
	if path == "/effects" {
		return auth.RoleOperator
	}
	if strings.HasPrefix(path, "/queues/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
		return auth.RoleAdmin
	}
	if path == "/stats" || path == "/metrics" || path == "/queues" || path == "/dlq" {
		return auth.RoleOperator
	}
	if path == "/tasks" && r.Method == http.MethodPost || strings.HasSuffix(path, "/requeue") || strings.HasSuffix(path, "/cancel") {
		return auth.RoleOperator
	}
	return auth.RoleReader
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != "" && !strings.ContainsAny(token, " \t\r\n")
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	contract.WriteError(w, r, domain.NewError(domain.ErrorKindUnauthorized, "unauthorized", "authentication is required", nil))
}

func isAdminMutation(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	return strings.HasPrefix(path, "/queues/") || strings.HasPrefix(path, "/effects/") || strings.HasSuffix(path, "/requeue") || strings.HasSuffix(path, "/cancel")
}

func isSensitiveEndpoint(r *http.Request) bool {
	return strings.HasPrefix(strings.TrimPrefix(r.URL.Path, "/v1"), "/effects")
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
