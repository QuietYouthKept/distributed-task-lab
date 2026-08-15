package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

func replayPayload(candidateRatio float64, caseCount int) map[string]any {
	return map[string]any{
		"job_schema_version":   "1",
		"proposal_id":          "p-123",
		"proposal_version":     1,
		"strategy_key":         "new_user_feed_exploration",
		"baseline_config":      map[string]any{"exploration_ratio": 0.15},
		"candidate_config":     map[string]any{"exploration_ratio": candidateRatio},
		"scenario_set":         "default",
		"scenario_set_version": "v1",
		"simulator_version":    "go-v1",
		"policy_version":       "v1",
		"seed":                 42,
		"case_count":           caseCount,
		"guardrail_config":     map[string]any{"negative_feedback_delta": 0.02},
	}
}

func parseResult(t *testing.T, r *worker.TaskExecResult) map[string]any {
	t.Helper()
	raw := r.Artifacts["replay_result"]
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("replay_result is not valid JSON: %v", err)
	}
	return m
}

func TestStrategyReplayDeterministic(t *testing.T) {
	h := NewStrategyReplay()
	a, err := h.Run(context.Background(), replayPayload(0.25, 1000))
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.Run(context.Background(), replayPayload(0.25, 1000))
	if err != nil {
		t.Fatal(err)
	}
	am, bm := parseResult(t, a), parseResult(t, b)
	for _, k := range []string{"passed", "failed", "guardrail_violations", "risk_summary", "result_status"} {
		if am[k] != bm[k] {
			t.Fatalf("field %s differs between runs: %v vs %v", k, am[k], bm[k])
		}
	}
}

func TestStrategyReplaySafe(t *testing.T) {
	h := NewStrategyReplay()
	r, err := h.Run(context.Background(), replayPayload(0.20, 1000))
	if err != nil {
		t.Fatal(err)
	}
	m := parseResult(t, r)
	if m["result_status"] != "PASS" {
		t.Fatalf("expected PASS, got %v", m["result_status"])
	}
	if m["passed"].(float64) != 1000 {
		t.Fatalf("expected 1000 passed, got %v", m["passed"])
	}
	if m["failed"].(float64) != 0 {
		t.Fatalf("expected 0 failed, got %v", m["failed"])
	}
}

func TestStrategyReplayRegression(t *testing.T) {
	h := NewStrategyReplay()
	r, err := h.Run(context.Background(), replayPayload(0.25, 1000))
	if err != nil {
		t.Fatal(err)
	}
	m := parseResult(t, r)
	if m["result_status"] != "REGRESSION" {
		t.Fatalf("expected REGRESSION, got %v", m["result_status"])
	}
	if m["failed"].(float64) <= 0 {
		t.Fatalf("expected failures > 0")
	}
	if m["guardrail_violations"].(float64) <= 0 {
		t.Fatalf("expected violations > 0")
	}
}

func TestStrategyReplayRegressionIsSuccessNotFailure(t *testing.T) {
	// A regression is a successfully-computed business result, not a system error.
	h := NewStrategyReplay()
	r, err := h.Run(context.Background(), replayPayload(0.25, 1000))
	if err != nil {
		t.Fatalf("regression must NOT be a handler error: %v", err)
	}
	m := parseResult(t, r)
	if m["result_status"] != "REGRESSION" {
		t.Fatalf("expected result_status=REGRESSION, got %v", m["result_status"])
	}
}

func TestStrategyReplayResultSelfContained(t *testing.T) {
	h := NewStrategyReplay()
	r, err := h.Run(context.Background(), replayPayload(0.20, 50))
	if err != nil {
		t.Fatal(err)
	}
	m := parseResult(t, r)
	for _, k := range []string{
		"proposal_id", "proposal_version", "scenario_set_version",
		"simulator_version", "policy_version", "seed", "case_count",
		"total_cases", "passed", "failed", "risk_summary", "result_status",
	} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing field %s in self-contained result", k)
		}
	}
}

func TestStrategyReplayBadPayloadNonRetryable(t *testing.T) {
	h := NewStrategyReplay()
	_, err := h.Run(context.Background(), map[string]any{"case_count": 10})
	if err == nil {
		t.Fatal("expected error for missing proposal_id")
	}
	var ee *worker.ExecutionError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExecutionError, got %T", err)
	}
	if ee.Retryable {
		t.Fatal("bad payload must be non-retryable (straight to DLQ)")
	}
}
