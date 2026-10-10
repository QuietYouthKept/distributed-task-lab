package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/application"
	"github.com/fantasticyi/distributed-task-lab/internal/auth"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TaskHandler handles HTTP requests for tasks.
type TaskHandler struct {
	svc    *application.TaskService
	logger *slog.Logger
	cursor taskCursorCodec
}

// NewTaskHandler creates a new task HTTP handler.
func NewTaskHandler(svc *application.TaskService, logger *slog.Logger) *TaskHandler {
	codec, _ := newCursorCodec([]byte("development-only-cursor-signing-key-32-bytes"))
	return &TaskHandler{svc: svc, logger: logger, cursor: codec}
}

// NewTaskHandlerWithCursorKey constructs the v1 pagination signer from the
// deployment secret. Production callers must supply at least 32 random bytes.
func NewTaskHandlerWithCursorKey(svc *application.TaskService, logger *slog.Logger, key []byte) (*TaskHandler, error) {
	codec, err := newCursorCodec(key)
	if err != nil {
		return nil, err
	}
	return &TaskHandler{svc: svc, logger: logger, cursor: codec}, nil
}

// NewTaskHandlerWithCursorKeyRing explicitly opts into expiring v2 cursors.
// NewTaskHandlerWithCursorKey remains the compatibility-preserving default.
func NewTaskHandlerWithCursorKeyRing(svc *application.TaskService, logger *slog.Logger, config CursorKeyRingConfig) (*TaskHandler, error) {
	codec, err := newRotatingCursorCodec(config)
	if err != nil {
		return nil, err
	}
	return &TaskHandler{svc: svc, logger: logger, cursor: codec}, nil
}

// CreateTask handles POST /tasks.
func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req application.CreateTaskRequest
	if err := decodeRequest(r, &req); err != nil {
		writeMalformedRequest(w, r, "invalid_json", "request body must be valid JSON")
		return
	}

	// Default to rag_ingestion if type not specified.
	if req.Type == "" {
		req.Type = domain.TaskTypeRAGIngestion
	}
	if req.Payload.Data == nil {
		req.Payload.Data = map[string]any{}
	}
	if err := applyIdempotencyKey(r, &req); err != nil {
		writeError(w, r, err)
		return
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
		writeError(w, r, err)
		return
	}

	h.logger.Info("task created", "task_id", resp.ID, "type", req.Type)
	writeJSON(w, http.StatusCreated, resp)
}

// GetTask handles GET /tasks/{id}.
func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeInvalidRequest(w, r, "missing_task_id", "task id is required")
		return
	}

	task, err := h.svc.GetTask(r.Context(), id)
	if err != nil {
		h.logger.Error("get task failed", "error", err)
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// ListTasks handles GET /tasks.
func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	v1 := r.URL.Path == "/v1/tasks"
	pageSize := 100
	if v1 {
		pageSize = 50
	}
	filter := domain.TaskFilter{Limit: pageSize}

	// Parse query parameters.
	q := r.URL.Query()
	if statusStr := q.Get("status"); statusStr != "" {
		st := domain.TaskStatus(statusStr)
		if !domain.ValidTaskStatuses[st] {
			writeInvalidRequest(w, r, "invalid_filter", "status filter is not supported")
			return
		}
		filter.Status = &st
	}
	if typeStr := q.Get("type"); typeStr != "" {
		tt := domain.TaskType(typeStr)
		if !domain.ValidTaskTypes[tt] {
			writeInvalidRequest(w, r, "invalid_filter", "type filter is not supported")
			return
		}
		filter.Type = &tt
	}
	if queueName := q.Get("queue"); queueName != "" {
		if len(queueName) > 128 || strings.TrimSpace(queueName) != queueName {
			writeInvalidRequest(w, r, "invalid_filter", "queue filter is invalid")
			return
		}
		filter.Queue = &queueName
	}
	if v1 {
		if raw := q.Get("page_size"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeInvalidRequest(w, r, "invalid_page_size", "page_size must be between 1 and 100")
				return
			}
			pageSize = parsed
		}
		principal, _ := auth.PrincipalFromContext(r.Context())
		subject := principal.Subject
		status, taskType, queueName := q.Get("status"), q.Get("type"), q.Get("queue")
		fingerprint := taskFilterFingerprint(status, taskType, queueName, subject)
		if token := q.Get("cursor"); token != "" {
			if h.cursor == nil {
				writeError(w, r, domain.NewError(domain.ErrorKindInternal, "pagination_unavailable", "pagination is unavailable", nil))
				return
			}
			createdAt, id, err := h.cursor.decode(token, fingerprint, pageSize, subject)
			if err != nil {
				writeInvalidRequest(w, r, "invalid_cursor", "cursor is invalid or does not match the query")
				return
			}
			filter.AfterCreatedAt, filter.AfterID = &createdAt, &id
		}
		filter.Limit = pageSize + 1
	}

	tasks, err := h.svc.ListTasks(r.Context(), filter)
	if err != nil {
		h.logger.Error("list tasks failed", "error", err)
		writeError(w, r, err)
		return
	}

	if !v1 {
		writeJSON(w, http.StatusOK, tasks)
		return
	}
	hasMore := len(tasks) > pageSize
	if hasMore {
		tasks = tasks[:pageSize]
	}
	next := ""
	if hasMore && len(tasks) > 0 {
		last := tasks[len(tasks)-1]
		principal, _ := auth.PrincipalFromContext(r.Context())
		status, taskType, queueName := q.Get("status"), q.Get("type"), q.Get("queue")
		next = h.cursor.encode(taskCursor{Version: 1, CreatedAt: last.CreatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID,
			Filter: taskFilterFingerprint(status, taskType, queueName, principal.Subject), PageSize: pageSize, Subject: principal.Subject})
	}
	writeJSON(w, http.StatusOK, contract.TaskListResponse{Tasks: tasks, NextCursor: next, HasMore: hasMore})
}

// ListDLQ handles GET /tasks?status=dlq (also supports /dlq for convenience).
func (h *TaskHandler) ListDLQ(w http.ResponseWriter, r *http.Request) {
	filter := domain.TaskFilter{
		Limit: 100,
	}

	tasks, err := h.svc.ListDLQ(r.Context(), filter)
	if err != nil {
		h.logger.Error("list dlq failed", "error", err)
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// GetStats returns task counts by status using a single aggregate query.
func (h *TaskHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	counts, err := h.svc.GetStats(r.Context())
	if err != nil {
		h.logger.Error("stats query failed", "error", err)
		writeError(w, r, err)
		return
	}

	// Emit every known status so the JSON shape stays stable for callers.
	out := make(contract.TaskStatsResponse, len(domain.ValidTaskStatuses))
	for st := range domain.ValidTaskStatuses {
		out[st] = counts[st]
	}

	writeJSON(w, http.StatusOK, out)
}

// Requeue handles POST /tasks/{id}/requeue — resets a DLQ task to pending.
func (h *TaskHandler) Requeue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeInvalidRequest(w, r, "missing_task_id", "task id is required")
		return
	}

	var body contract.OptionalReasonRequest
	if err := decodeOptionalRequest(r, &body); err != nil {
		writeMalformedRequest(w, r, "invalid_json", "request body must be valid JSON")
		return
	}
	if body.Reason == "" {
		body.Reason = "manual requeue"
	}

	if err := h.svc.Requeue(r.Context(), id, body.Reason); err != nil {
		h.logger.Error("requeue failed", "task_id", id, "error", err)
		writeError(w, r, err)
		return
	}

	h.logger.Info("task requeued", "task_id", id)
	writeJSON(w, http.StatusOK, contract.TaskActionResponse{Status: "requeued", TaskID: id})
}

// Cancel handles POST /tasks/{id}/cancel — cancels a pending or running task.
func (h *TaskHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeInvalidRequest(w, r, "missing_task_id", "task id is required")
		return
	}

	var body contract.OptionalReasonRequest
	if err := decodeOptionalRequest(r, &body); err != nil {
		writeMalformedRequest(w, r, "invalid_json", "request body must be valid JSON")
		return
	}
	if body.Reason == "" {
		body.Reason = "manual cancel"
	}

	if err := h.svc.Cancel(r.Context(), id, body.Reason); err != nil {
		h.logger.Error("cancel failed", "task_id", id, "error", err)
		writeError(w, r, err)
		return
	}

	h.logger.Info("task cancelled", "task_id", id)
	writeJSON(w, http.StatusOK, contract.TaskActionResponse{Status: "cancelled", TaskID: id})
}

// GetTaskEvents handles GET /tasks/{id}/events — returns the task audit log.
func (h *TaskHandler) GetTaskEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeInvalidRequest(w, r, "missing_task_id", "task id is required")
		return
	}

	events, err := h.svc.GetTaskEvents(r.Context(), id)
	if err != nil {
		h.logger.Error("get events failed", "task_id", id, "error", err)
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, events)
}

// PauseQueue handles POST /queues/{name}/pause.
func (h *TaskHandler) PauseQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeInvalidRequest(w, r, "missing_queue", "queue name is required")
		return
	}
	if err := h.svc.PauseQueue(r.Context(), name); err != nil {
		h.logger.Error("pause queue failed", "queue", name, "error", err)
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.QueueActionResponse{Status: "paused", Queue: name})
}

// ResumeQueue handles POST /queues/{name}/resume.
func (h *TaskHandler) ResumeQueue(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeInvalidRequest(w, r, "missing_queue", "queue name is required")
		return
	}
	if err := h.svc.ResumeQueue(r.Context(), name); err != nil {
		h.logger.Error("resume queue failed", "queue", name, "error", err)
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.QueueActionResponse{Status: "resumed", Queue: name})
}

// ListQueues handles GET /queues.
func (h *TaskHandler) ListQueues(w http.ResponseWriter, r *http.Request) {
	queues, err := h.svc.ListQueues(r.Context())
	if err != nil {
		h.logger.Error("list queues failed", "error", err)
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, queues)
}

// SetQueueConfig handles PUT /queues/{name}.
func (h *TaskHandler) SetQueueConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeInvalidRequest(w, r, "missing_queue", "queue name is required")
		return
	}
	var body contract.QueueConfigRequest
	if err := decodeOptionalRequest(r, &body); err != nil {
		writeMalformedRequest(w, r, "invalid_json", "request body must be valid JSON")
		return
	}

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
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.QueueActionResponse{Status: "configured", Queue: name})
}

func applyIdempotencyKey(r *http.Request, req *application.CreateTaskRequest) error {
	values := r.Header.Values("Idempotency-Key")
	if len(values) == 0 {
		return nil
	}
	if len(values) != 1 {
		return domain.InvalidArgument("invalid_idempotency_key", "exactly one Idempotency-Key header is allowed", nil)
	}

	key := values[0]
	if key == "" || len(key) > 256 || strings.TrimSpace(key) != key {
		return domain.InvalidArgument("invalid_idempotency_key", "Idempotency-Key must be between 1 and 256 bytes without surrounding whitespace", nil)
	}
	if req.IdempotencyKey != nil && *req.IdempotencyKey != key {
		return domain.InvalidArgument("idempotency_key_mismatch", "Idempotency-Key header and body value must match", nil)
	}
	req.IdempotencyKey = &key
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
