package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

func TestMockHandlerSuccess(t *testing.T) {
	h := NewMockHandler()
	res, err := h.Handle(context.Background(), "rag_ingestion",
		map[string]any{"document_id": "doc-1", "mock_duration_ms": 1}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.Summary == "" {
		t.Error("expected a non-empty result")
	}
}

func TestMockHandlerRetryThenSuccess(t *testing.T) {
	h := NewMockHandler()
	ctx := context.Background()

	_, err := h.Handle(ctx, "rag_ingestion", map[string]any{"simulate": "retry_then_success", "mock_duration_ms": 1}, 1)
	if err == nil {
		t.Error("attempt 1 should fail")
	}
	res, err := h.Handle(ctx, "rag_ingestion", map[string]any{"simulate": "retry_then_success", "mock_duration_ms": 1}, 2)
	if err != nil {
		t.Fatalf("attempt 2 should succeed: %v", err)
	}
	if res == nil {
		t.Error("expected a result on attempt 2")
	}
}

func TestMockHandlerNonRetryable(t *testing.T) {
	h := NewMockHandler()

	_, err := h.Handle(context.Background(), "rag_ingestion",
		map[string]any{"simulate": "validation_error", "mock_duration_ms": 1}, 1)

	var ee *worker.ExecutionError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *worker.ExecutionError, got %v", err)
	}
	if ee.Retryable {
		t.Error("expected Retryable=false")
	}
	if ee.ErrorType != domain.ErrorTypeValidation {
		t.Errorf("ErrorType = %q, want %q", ee.ErrorType, domain.ErrorTypeValidation)
	}
}

func TestMockHandlerTimeout(t *testing.T) {
	h := NewMockHandler()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := h.Handle(ctx, "rag_ingestion", map[string]any{"simulate": "timeout"}, 1)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestProductionExecutionPolicyRejectsValidMockPayload(t *testing.T) {
	h := NewCompositeWithAdmission(t.TempDir(), t.TempDir(), taskpayload.ProductionExecutionPolicy())
	_, err := h.Handle(context.Background(), "rag_ingestion", map[string]any{
		"execution_mode": "mock", "document_id": "doc-1",
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "not enabled in this worker deployment") {
		t.Fatalf("valid but disabled mock mode was not rejected: %v", err)
	}
}

func TestStagingSimulationPolicyRejectsMockAndNonGatewayEval(t *testing.T) {
	h := NewCompositeWithAdmission(t.TempDir(), t.TempDir(), taskpayload.StagingSimulationExecutionPolicy())
	tests := []struct {
		name     string
		taskType string
		payload  map[string]any
	}{
		{"mock task", "rag_ingestion", map[string]any{"execution_mode": "mock", "document_id": "doc-1"}},
		{"deterministic eval", "eval", map[string]any{"execution_mode": "deterministic", "dataset_version": "d", "model_profile": "m", "prompt_version": "p", "run_id": "r", "case_start": 0, "case_end": 1}},
		{"payment effect", "payment", map[string]any{"execution_mode": "effect", "operation": "payment", "operation_id": "op-1", "target": "target-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.Handle(context.Background(), tt.taskType, tt.payload, 1)
			if err == nil || !strings.Contains(err.Error(), "not enabled in this worker deployment") {
				t.Fatalf("unapproved work should be rejected by the worker policy, got %v", err)
			}
		})
	}
}

func TestStagingSimulationPolicyRejectsNotification(t *testing.T) {
	h := NewCompositeWithAdmission(t.TempDir(), t.TempDir(), taskpayload.StagingSimulationExecutionPolicy())
	_, err := h.Handle(context.Background(), "notification", map[string]any{
		"execution_mode": "effect", "operation": "notification", "operation_id": "notice-1", "target": "target-1",
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "not enabled in this worker deployment") {
		t.Fatalf("notification must remain disabled in staging simulation, got %v", err)
	}
}
