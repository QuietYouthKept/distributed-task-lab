package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

// StatsProvider returns the number of tasks currently in each status.
// It is a single aggregate query rather than a per-status list scan.
type StatsProvider func(ctx context.Context) (map[domain.TaskStatus]int64, error)

// MetricsHandler serves Prometheus-format metrics.
type MetricsHandler struct {
	stats StatsProvider
}

// NewMetricsHandler creates a metrics handler.
func NewMetricsHandler(stats StatsProvider) *MetricsHandler {
	return &MetricsHandler{stats: stats}
}

// ServeMetrics handles GET /metrics — Prometheus text format.
func (h *MetricsHandler) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	counts, err := h.stats(r.Context())
	if err != nil {
		http.Error(w, "metrics error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP tasklab_tasks_total Task count by status.\n")
	fmt.Fprintf(w, "# TYPE tasklab_tasks_total gauge\n")
	for _, st := range []domain.TaskStatus{
		domain.TaskStatusPending, domain.TaskStatusRunning,
		domain.TaskStatusSuccess, domain.TaskStatusFailed, domain.TaskStatusDLQ,
	} {
		fmt.Fprintf(w, "tasklab_tasks_total{status=%q} %d\n", string(st), counts[st])
	}
}
