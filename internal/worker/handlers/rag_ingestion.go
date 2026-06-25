package handlers

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/yourusername/distributed-task-lab/internal/worker"
)

// MockHandler implements worker.Handler with mock task behaviors.
// Supports simulating success, failure, retry, and timeout via payload parameters.
//
// Payload simulation keys:
//   mock_duration_ms           — simulated processing time in milliseconds (default 500)
//   mock_failures_before_success — fail this many times before succeeding (uses attempt)
//   mock_always_fail            — always fail regardless of attempt
//   simulate                    — shortcut: "success", "failure", "timeout", "retry_then_success"
//
// The simulate shortcuts take precedence over the fine-grained controls.
type MockHandler struct {
	rng *rand.Rand
}

// NewMockHandler creates a new mock task handler.
func NewMockHandler() *MockHandler {
	return &MockHandler{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Handle executes the mock task based on its type and payload.
func (h *MockHandler) Handle(ctx context.Context, taskType string, payload map[string]any, attempt int) (*worker.TaskExecResult, error) {
	// Read simulation parameters.
	simulate := getString(payload, "simulate", "")
	durationMs := getInt(payload, "mock_duration_ms", 500)
	if d := getInt(payload, "duration_ms", 0); d > 0 && durationMs == 500 {
		durationMs = d // backward compat
	}
	failuresBeforeSuccess := getInt(payload, "mock_failures_before_success", -1)
	alwaysFail := getBool(payload, "mock_always_fail", false)

	// Apply simulated processing duration.
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("task cancelled or timed out: %w", ctx.Err())
	case <-time.After(time.Duration(durationMs) * time.Millisecond):
	}

	// Simulation shortcuts (take precedence).
	switch simulate {
	case "timeout":
		// Sleep until context deadline expires.
		<-ctx.Done()
		return nil, fmt.Errorf("simulated timeout: %w", ctx.Err())

	case "failure":
		return nil, fmt.Errorf("simulated failure for task type %s", taskType)

	case "retry_then_success":
		// Fail on attempt 1, succeed on attempt 2+.
		if attempt <= 1 {
			return nil, fmt.Errorf("simulated retry: attempt %d failed", attempt)
		}
		// fall through to success
	}

	// Fine-grained controls.
	if alwaysFail {
		return nil, fmt.Errorf("mock_always_fail=true on attempt %d", attempt)
	}

	if failuresBeforeSuccess >= 0 && attempt <= failuresBeforeSuccess {
		return nil, fmt.Errorf("mock failure: attempt %d <= failures_before_success %d", attempt, failuresBeforeSuccess)
	}

	// Success path.
	var result *worker.TaskExecResult
	switch taskType {
	case "rag_ingestion":
		result = h.mockRAGIngestion(payload, attempt)
	case "eval":
		result = h.mockEval(payload)
	case "benchmark":
		result = h.mockBenchmark(payload)
	default:
		result = h.mockGeneric(taskType, payload)
	}

	return result, nil
}

func (h *MockHandler) mockRAGIngestion(payload map[string]any, attempt int) *worker.TaskExecResult {
	docID := getString(payload, "document_id", "doc-unknown")
	datasetID := getString(payload, "dataset_id", "ds-default")
	chunksGenerated := h.rng.Intn(50) + 10

	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("RAG ingestion completed for document %s in dataset %s (attempt %d)", docID, datasetID, attempt),
		Artifacts: map[string]string{
			"document_id":     docID,
			"dataset_id":      datasetID,
			"index_name":      getString(payload, "index_name", "mock-index"),
			"chunk_count":     fmt.Sprintf("%d", chunksGenerated),
			"embedding_model": getString(payload, "embedding_profile", "text-embedding-mock"),
		},
		Metrics: map[string]float64{
			"processed_documents": 1,
			"generated_chunks":    float64(chunksGenerated),
			"duration_ms":         float64(h.rng.Intn(500) + 100),
		},
	}
}

func (h *MockHandler) mockEval(payload map[string]any) *worker.TaskExecResult {
	evalSetID := getString(payload, "eval_set_id", "eval-default")
	model := getString(payload, "model_profile", "mock-model")

	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("Evaluation completed for eval set %s using model %s", evalSetID, model),
		Artifacts: map[string]string{
			"eval_set_id": evalSetID,
			"model":       model,
		},
		Metrics: map[string]float64{
			"accuracy":    0.85 + h.rng.Float64()*0.1,
			"latency_ms":  float64(h.rng.Intn(2000) + 500),
		},
	}
}

func (h *MockHandler) mockBenchmark(payload map[string]any) *worker.TaskExecResult {
	benchName := getString(payload, "benchmark_name", "bench-default")
	concurrency := getInt(payload, "concurrency", 10)

	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("Benchmark %s completed with concurrency=%d", benchName, concurrency),
		Artifacts: map[string]string{
			"benchmark_name": benchName,
		},
		Metrics: map[string]float64{
			"throughput_rps": float64(h.rng.Intn(500) + 100),
			"p99_latency_ms": float64(h.rng.Intn(500) + 50),
		},
	}
}

func (h *MockHandler) mockGeneric(taskType string, payload map[string]any) *worker.TaskExecResult {
	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("Task type %s completed successfully", taskType),
		Artifacts: map[string]string{
			"task_type": taskType,
		},
		Metrics: map[string]float64{
			"duration_ms": float64(h.rng.Intn(1000) + 50),
		},
	}
}

// --- Payload helpers ---

func getString(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func getInt(m map[string]any, key string, fallback int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return fallback
}

func getBool(m map[string]any, key string, fallback bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}
