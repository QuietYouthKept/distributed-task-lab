package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
)

func TestEffectHandlerUnavailableUsesStableErrorEnvelope(t *testing.T) {
	handler := NewEffectHandler(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tests := []struct {
		name string
		run  func(http.ResponseWriter, *http.Request)
	}{
		{"list", handler.List},
		{"resolve", handler.Resolve},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/effects", nil)
			req = req.WithContext(contract.WithRequestID(context.Background(), "wave4-request-0001"))
			rec := httptest.NewRecorder()
			test.run(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s, want 503", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type=%q, want application/json", got)
			}
			var response contract.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v; body=%s", err, rec.Body.String())
			}
			if response.Error.Code != "effect_reconciliation_unavailable" || response.RequestID != "wave4-request-0001" {
				t.Fatalf("response=%+v, want stable unavailable envelope and request ID", response)
			}
		})
	}
}
