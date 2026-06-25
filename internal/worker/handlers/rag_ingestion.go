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
// It supports simulating success, failure, retry, and timeout
// based on payload parameters.
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
func (h *MockHandler) Handle(ctx context.Context, taskType string, payload map[string]any) (*worker.TaskExecResult, error) {
	// Check for simulation directives in the payload.
	simulate := getString(payload, "simulate", "success")
	durationMs := getInt(payload, "duration_ms", 500)
	shouldFail := getBool(payload, "should_fail", false)
	failOnAttempt := getInt(payload, "fail_on_attempt", -1)

	// Apply simulated processing duration.
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("task cancelled or timed out: %w", ctx.Err())
	case <-time.After(time.Duration(durationMs) * time.Millisecond):
	}

	// Check simulation mode.
	switch simulate {
	case "timeout":
		// Sleep longer than the context deadline to force timeout.
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("simulated timeout: %w", ctx.Err())
		case <-time.After(5 * time.Minute):
		}
		return nil, errors.New("simulated timeout")

	case "failure":
		return nil, fmt.Errorf("simulated failure for task type %s", taskType)

	case "retry_then_success":
		// Get current attempt from context or payload (in real use, attempt is tracked by worker).
		// For MVP, we use a simplified model: fail_on_attempt controls which attempt fails.
		if failOnAttempt > 0 {
			// We can't know the exact attempt here — the queue handles retry.
			// For demo purposes, we check a simple counter in the payload.
			tryCount := getInt(payload, "_try_count", 1)
			if tryCount <= failOnAttempt {
				payload["_try_count"] = tryCount + 1
				return nil, fmt.Errorf("simulated retry: attempt %d failed", tryCount)
			}
		}
	}

	if shouldFail {
		return nil, fmt.Errorf("task marked as should_fail=true")
	}

	// Success path.
	var result *worker.TaskExecResult
	switch taskType {
	case "rag_ingestion":
		result = h.mockRAGIngestion(payload)
	case "eval":
		result = h.mockEval(payload)
	case "benchmark":
		result = h.mockBenchmark(payload)
	default:
		result = h.mockGeneric(taskType, payload)
	}

	return result, nil
}

func (h *MockHandler) mockRAGIngestion(payload map[string]any) *worker.TaskExecResult {
	docID := getString(payload, "document_id", "doc-unknown")
	datasetID := getString(payload, "dataset_id", "ds-default")
	chunksGenerated := h.rng.Intn(50) + 10

	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("RAG ingestion completed for document %s in dataset %s", docID, datasetID),
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
