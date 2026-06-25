package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/yourusername/distributed-task-lab/internal/api/handlers"
)

// NewRouter creates and configures the HTTP router.
// Uses Go 1.22+ enhanced ServeMux patterns.
func NewRouter(taskHandler *handlers.TaskHandler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Task endpoints — Go 1.22+ method+path patterns
	mux.HandleFunc("POST /tasks", taskHandler.CreateTask)
	mux.HandleFunc("GET /tasks", taskHandler.ListTasks)
	mux.HandleFunc("GET /tasks/{id}", taskHandler.GetTask)

	// DLQ convenience endpoint
	mux.HandleFunc("GET /dlq", taskHandler.ListDLQ)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Wrap with logging middleware
	return loggingMiddleware(mux, logger)
}

// loggingMiddleware logs each request.
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
