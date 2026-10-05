package handlers

import (
	"context"
	"os"
	"testing"
)

func evalPayload() map[string]any {
	return map[string]any{"dataset_version": "golden-2026-10", "model_profile": "mock-gpt", "prompt_version": "v3", "run_id": "release-gate", "case_start": 0, "case_end": 100}
}

func TestLLMEvalWritesAndReusesShard(t *testing.T) {
	h := NewLLMEval(t.TempDir())
	first, err := h.Run(context.Background(), evalPayload())
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifacts["idempotent_reused"] != "false" {
		t.Fatal("first run should create the shard")
	}
	if _, err := os.Stat(first.Artifacts["case_results_uri"]); err != nil {
		t.Fatal(err)
	}
	second, err := h.Run(context.Background(), evalPayload())
	if err != nil {
		t.Fatal(err)
	}
	if second.Artifacts["idempotent_reused"] != "true" || second.Artifacts["result_fingerprint"] != first.Artifacts["result_fingerprint"] {
		t.Fatal("retry must reuse the same shard")
	}
}

func TestLLMEvalRejectsIncompletePayload(t *testing.T) {
	if _, err := NewLLMEval(t.TempDir()).Run(context.Background(), map[string]any{"run_id": "x"}); err == nil {
		t.Fatal("expected validation error")
	}
}
