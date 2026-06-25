package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/yourusername/distributed-task-lab/internal/application"
	"github.com/yourusername/distributed-task-lab/internal/domain"
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

	resp, err := h.svc.CreateTask(r.Context(), req)
	if err != nil {
		h.logger.Error("create task failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
