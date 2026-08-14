package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
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
