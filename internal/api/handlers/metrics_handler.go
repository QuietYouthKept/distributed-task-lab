package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

// MetricsProvider supplies the data behind /metrics.
type MetricsProvider interface {
	GetStats(ctx context.Context) (map[domain.TaskStatus]int64, error)
	GetQueueDepth(ctx context.Context) (map[string]map[domain.TaskStatus]int64, error)
	GetOldestPendingAge(ctx context.Context) (time.Duration, error)
}

// MetricsHandler serves Prometheus-format metrics.
type MetricsHandler struct {
	provider MetricsProvider
}

// NewMetricsHandler creates a metrics handler.
func NewMetricsHandler(p MetricsProvider) *MetricsHandler {
	return &MetricsHandler{provider: p}
}

// ServeMetrics handles GET /metrics — Prometheus text format.
func (h *MetricsHandler) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	counts, err := h.provider.GetStats(ctx)
	if err != nil {
		http.Error(w, "metrics error", http.StatusInternalServerError)
		return
	}
	depth, err := h.provider.GetQueueDepth(ctx)
	if err != nil {
		http.Error(w, "metrics error", http.StatusInternalServerError)
		return
	}
	oldest, err := h.provider.GetOldestPendingAge(ctx)
	if err != nil {
		http.Error(w, "metrics error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP tasklab_tasks_total Task count by status.\n")
	fmt.Fprintf(w, "# TYPE tasklab_tasks_total gauge\n")
	for _, st := range allStatuses() {
		fmt.Fprintf(w, "tasklab_tasks_total{status=%q} %d\n", string(st), counts[st])
	}

	fmt.Fprintf(w, "# HELP tasklab_queue_depth Task count by queue and status.\n")
	fmt.Fprintf(w, "# TYPE tasklab_queue_depth gauge\n")
	for queue, byStatus := range depth {
		for _, st := range allStatuses() {
			fmt.Fprintf(w, "tasklab_queue_depth{queue=%q,status=%q} %d\n", queue, string(st), byStatus[st])
		}
	}

	fmt.Fprintf(w, "# HELP tasklab_oldest_pending_age_seconds Age of the oldest pending task.\n")
	fmt.Fprintf(w, "# TYPE tasklab_oldest_pending_age_seconds gauge\n")
	fmt.Fprintf(w, "tasklab_oldest_pending_age_seconds %d\n", int64(oldest.Seconds()))
}

func allStatuses() []domain.TaskStatus {
	return []domain.TaskStatus{
		domain.TaskStatusPending, domain.TaskStatusRunning,
		domain.TaskStatusSuccess, domain.TaskStatusFailed, domain.TaskStatusDLQ,
	}
}
