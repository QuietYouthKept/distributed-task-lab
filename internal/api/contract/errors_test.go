package contract

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestClassifyErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"malformed request", domain.NewError(domain.ErrorKindMalformedRequest, "invalid_json", "invalid JSON", nil), http.StatusBadRequest, "invalid_json"},
		{"validation", domain.InvalidArgument("invalid_task_type", "unsupported", nil), http.StatusUnprocessableEntity, "invalid_task_type"},
		{"unauthorized", domain.NewError(domain.ErrorKindUnauthorized, "unauthorized", "auth required", nil), http.StatusUnauthorized, "unauthorized"},
		{"forbidden", domain.NewError(domain.ErrorKindForbidden, "forbidden", "not allowed", nil), http.StatusForbidden, "forbidden"},
		{"not found", domain.ErrTaskNotFound, http.StatusNotFound, "not_found"},
		{"conflict", domain.ErrTaskStateConflict, http.StatusConflict, "state_conflict"},
		{"unavailable", domain.NewError(domain.ErrorKindUnavailable, "database_unavailable", "retry later", nil), http.StatusServiceUnavailable, "database_unavailable"},
		{"internal", errors.New("pq: password=top-secret"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := ClassifyError(tt.err)
			if status != tt.want || body.Code != tt.code {
				t.Fatalf("ClassifyError() = (%d, %+v), want (%d, %q)", status, body, tt.want, tt.code)
			}
		})
	}
}

func TestWriteErrorDoesNotLeakCauseAndCarriesRequestID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/tasks", nil).WithContext(WithRequestID(context.Background(), "request-123456789"))
	rec := httptest.NewRecorder()
	WriteError(rec, req, errors.New("pq: password=top-secret"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "top-secret") {
		t.Fatalf("unexpected response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.RequestID != "request-123456789" || response.Error.Code != "internal_error" {
		t.Fatalf("response=%+v", response)
	}
}
