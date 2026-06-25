package handlers

import (
	"fmt"
	"net/http"

	"github.com/yourusername/distributed-task-lab/internal/domain"
)

// MetricsHandler serves Prometheus-format metrics.
type MetricsHandler struct {
	// taskLister is a function that returns all tasks for metrics calculation.
	taskLister func(status *domain.TaskStatus) ([]domain.Task, error)
}

// NewMetricsHandler creates a metrics handler.
func NewMetricsHandler(taskLister func(status *domain.TaskStatus) ([]domain.Task, error)) *MetricsHandler {
	return &MetricsHandler{taskLister: taskLister}
}

// ServeMetrics handles GET /metrics — Prometheus text format.
func (h *MetricsHandler) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	counts := map[domain.TaskStatus]int{}
	for _, st := range []domain.TaskStatus{
		domain.TaskStatusPending, domain.TaskStatusRunning,
		domain.TaskStatusSuccess, domain.TaskStatusFailed, domain.TaskStatusDLQ,
	} {
		tasks, err := h.taskLister(&st)
		if err != nil {
			http.Error(w, "metrics error", http.StatusInternalServerError)
			return
		}
		counts[st] = len(tasks)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP tasklab_tasks_total Task count by status.\n")
	fmt.Fprintf(w, "# TYPE tasklab_tasks_total gauge\n")
	for _, st := range []domain.TaskStatus{
		domain.TaskStatusPending, domain.TaskStatusRunning,
		domain.TaskStatusSuccess, domain.TaskStatusFailed, domain.TaskStatusDLQ,
	} {
		fmt.Fprintf(w, "tasklab_tasks_total{status=\"%s\"} %d\n", st, counts[st])
	}
}
