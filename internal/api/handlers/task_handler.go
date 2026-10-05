package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/fantasticyi/distributed-task-lab/internal/application"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TaskHandler handles HTTP requests for tasks.
type TaskHandler struct {
	svc    *application.TaskService
	logger *slog.Logger
}

// NewTaskHandler creates a new task HTTP handler.
func NewTaskHandler(svc *application.TaskService, logger *slog.Logger) *TaskHandler {
	return &TaskHandler{svc: svc, logger: logger}
}

// CreateTask handles POST /tasks.
func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req application.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	// Default to rag_ingestion if type not specified.
	if req.Type == "" {
		req.Type = domain.TaskTypeRAGIngestion
	}
	if req.Payload.Data == nil {
		req.Payload.Data = map[string]any{}
	}
	// Trace propagation keys are service-owned. Discard client-supplied values
	// before injecting the authenticated HTTP span so callers cannot forge a
	// parent for worker-side telemetry.
	delete(req.Payload.Data, "_traceparent")
	delete(req.Payload.Data, "_tracestate")
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(r.Context(), carrier)
	if traceparent := carrier.Get("traceparent"); traceparent != "" {
		req.Payload.Data["_traceparent"] = traceparent
	}
	if tracestate := carrier.Get("tracestate"); tracestate != "" {
		req.Payload.Data["_tracestate"] = tracestate
	}

	resp, err := h.svc.CreateTask(r.Context(), req)
	if err != nil {
		h.logger.Error("create task failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	h.logger.Info("task created", "task_id", resp.ID, "type", req.Type)
	writeJSON(w, http.StatusCreated, resp)
}

// GetTask handles GET /tasks/{id}.
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing task id"})
		return
	}

	task, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		if err == domain.ErrTaskNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		h.logger.Error("get task failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// ListTasks handles GET /tasks.
func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	filter := domain.TaskFilter{
		Limit: 100,
	}

	// Parse query parameters.
	q := r.URL.Query()
	if statusStr := q.Get("status"); statusStr != "" {
		st := domain.TaskStatus(statusStr)
		if domain.ValidTaskStatuses[st] {
			filter.Status = &st
		}
	}
	if typeStr := q.Get("type"); typeStr != "" {
		tt := domain.TaskType(typeStr)
		if domain.ValidTaskTypes[tt] {
			filter.Type = &tt
		}
	}

	tasks, err := h.svc.ListTasks(r.Context(), filter)
	if err != nil {
		h.logger.Error("list tasks failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// ListDLQ handles GET /tasks?status=dlq (also supports /dlq for convenience).
func (h *TaskHandler) ListDLQ(w http.ResponseWriter, r *http.Request) {
	filter := domain.TaskFilter{
		Limit: 100,
	}

	tasks, err := h.svc.ListDLQ(r.Context(), filter)
	if err != nil {
		h.logger.Error("list dlq failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// GetStats returns task counts by status using a single aggregate query.
func (h *TaskHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	counts, err := h.svc.GetStats(r.Context())
	if err != nil {
		h.logger.Error("stats query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	// Emit every known status so the JSON shape stays stable for callers.
	out := make(map[string]int64, len(domain.ValidTaskStatuses))
	for st := range domain.ValidTaskStatuses {
		out[string(st)] = counts[st]
	}

	writeJSON(w, http.StatusOK, out)
}

// Requeue handles POST /tasks/{id}/requeue — resets a DLQ task to pending.
func (h *TaskHandler) Requeue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing task id"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Reason == "" {
		body.Reason = "manual requeue"
	}

	if err := h.svc.Requeue(r.Context(), id, body.Reason); err != nil {
		h.logger.Error("requeue failed", "task_id", id, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	h.logger.Info("task requeued", "task_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "requeued", "task_id": id})
}

// Cancel handles POST /tasks/{id}/cancel — cancels a pending or running task.
func (h *TaskHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing task id"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Reason == "" {
		body.Reason = "manual cancel"
	}

	if err := h.svc.Cancel(r.Context(), id, body.Reason); err != nil {
		h.logger.Error("cancel failed", "task_id", id, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	h.logger.Info("task cancelled", "task_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "task_id": id})
}

// GetTaskEvents handles GET /tasks/{id}/events — returns the task audit log.
func (h *TaskHandler) GetTaskEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing task id"})
		return
	}

	events, err := h.svc.GetTaskEvents(r.Context(), id)
	if err != nil {
		h.logger.Error("get events failed", "task_id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, events)
}

// PauseQueue handles POST /queues/{name}/pause.
func (h *TaskHandler) PauseQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing queue name"})
		return
	}
	if err := h.svc.PauseQueue(r.Context(), name); err != nil {
		h.logger.Error("pause queue failed", "queue", name, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "queue": name})
}

// ResumeQueue handles POST /queues/{name}/resume.
func (h *TaskHandler) ResumeQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing queue name"})
		return
	}
	if err := h.svc.ResumeQueue(r.Context(), name); err != nil {
		h.logger.Error("resume queue failed", "queue", name, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "queue": name})
}

// ListQueues handles GET /queues.
func (h *TaskHandler) ListQueues(w http.ResponseWriter, r *http.Request) {
	queues, err := h.svc.ListQueues(r.Context())
	if err != nil {
		h.logger.Error("list queues failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, queues)
}

// SetQueueConfig handles PUT /queues/{name}.
func (h *TaskHandler) SetQueueConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing queue name"})
		return
	}
	var body struct {
		MaxConcurrency     int     `json:"max_concurrency"`
		RateLimitPerSecond float64 `json:"rate_limit_per_second"`
		Burst              int     `json:"burst"`
		AgingAfterSeconds  int     `json:"aging_after_seconds"`
		AgingPriorityBoost int     `json:"aging_priority_boost"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	config := domain.QueueConfig{
		Name:               name,
		MaxConcurrency:     body.MaxConcurrency,
		RateLimitPerSecond: body.RateLimitPerSecond,
		Burst:              body.Burst,
		AgingAfterSeconds:  body.AgingAfterSeconds,
		AgingPriorityBoost: body.AgingPriorityBoost,
	}
	if err := h.svc.SetQueueConfig(r.Context(), config); err != nil {
		h.logger.Error("set queue config failed", "queue", name, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "configured", "queue": name})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
