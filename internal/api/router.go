package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/yourusername/distributed-task-lab/internal/api/handlers"
)

// NewRouter creates and configures the HTTP router.
// Uses Go 1.22+ enhanced ServeMux patterns.
func NewRouter(taskHandler *handlers.TaskHandler, metricsHandler *handlers.MetricsHandler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Task endpoints
	mux.HandleFunc("POST /tasks", taskHandler.CreateTask)
	mux.HandleFunc("GET /tasks", taskHandler.ListTasks)
	mux.HandleFunc("GET /tasks/{id}", taskHandler.GetTask)

	// DLQ convenience endpoint
	mux.HandleFunc("GET /dlq", taskHandler.ListDLQ)

	// Metrics endpoints
	mux.HandleFunc("GET /stats", taskHandler.GetStats)
	mux.HandleFunc("GET /metrics", metricsHandler.ServeMetrics)

	// Phase 6: Requeue, Cancel, Events
	mux.HandleFunc("POST /tasks/{id}/requeue", taskHandler.Requeue)
	mux.HandleFunc("POST /tasks/{id}/cancel", taskHandler.Cancel)
	mux.HandleFunc("GET /tasks/{id}/events", taskHandler.GetTaskEvents)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return loggingMiddleware(mux, logger)
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
