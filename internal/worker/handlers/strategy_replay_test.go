package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

func replayPayload(candidateRatio float64, caseCount int) map[string]any {
	return map[string]any{
		"job_schema_version":           "1",
		"replay_result_schema_version": "1",
		"proposal_id":                  "p-123",
		"proposal_version":             1,
		"strategy_key":                 "new_user_feed_exploration",
		"baseline_config":              map[string]any{"exploration_ratio": 0.15},
		"candidate_config":             map[string]any{"exploration_ratio": candidateRatio},
		"scenario_set":                 "default",
		"scenario_set_version":         "v1",
		"simulator_version":            "go-v1",
		"policy_version":               "v1",
		"seed":                         42,
		"case_count":                   caseCount,
		"guardrail_config":             map[string]any{"negative_feedback_delta": 0.02},
		"execution_fingerprint":        "abc123",
	}
}

func ragReplayPayload(topK float64, reranker bool, minScore float64, caseCount int) map[string]any {
	return map[string]any{
		"job_schema_version":           "1",
		"replay_result_schema_version": "1",
		"proposal_id":                  "rag-p-123",
		"proposal_version":             1,
		"strategy_key":                 "rag_retrieval_policy",
		"baseline_config":              map[string]any{"top_k": 8.0, "reranker_enabled": false, "min_score": 0.35},
		"candidate_config":             map[string]any{"top_k": topK, "reranker_enabled": reranker, "min_score": minScore},
		"scenario_set":                 "rag-golden",
		"scenario_set_version":         "v1",
		"simulator_version":            "go-v1",
		"policy_version":               "v1",
		"seed":                         42,
		"case_count":                   caseCount,
		"guardrail_config": map[string]any{
			"unsupported_claim_rate": map[string]any{"threshold": 0.0, "value_type": "delta", "severity": "HARD"},
			"p95_latency_ms":         map[string]any{"threshold": 0.20, "value_type": "relative_delta", "severity": "HARD"},
			"average_cost_usd":       map[string]any{"threshold": 0.25, "value_type": "relative_delta", "severity": "HARD"},
		},
		"execution_fingerprint":    "rag-fingerprint",
		"dataset_version":          "rag-golden-v1",
		"prompt_version":           "prompt-v1",
		"retriever_version":        "retriever-v1",
		"producer_commit":          strings.Repeat("a", 40),
		"evaluator_schema_version": "1",
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
		"replay_result_schema_version", "execution_fingerprint",
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

func TestStrategyReplayRAGWritesCompleteHashBoundEvidence(t *testing.T) {
	h := NewStrategyReplay(t.TempDir())
	r, err := h.Run(context.Background(), ragReplayPayload(10, true, 0.42, 100))
	if err != nil {
		t.Fatal(err)
	}
	m := parseResult(t, r)
	if m["result_status"] != "PASS" || m["failed"].(float64) != 0 {
		t.Fatalf("expected safe RAG PASS, got status=%v failed=%v", m["result_status"], m["failed"])
	}
	for _, key := range []string{
		"dataset_version", "prompt_version", "retriever_version", "producer_commit", "evaluator_schema_version",
		"recall_at_k", "mrr", "ndcg", "answer_point_recall", "unsupported_claim_rate", "refusal_accuracy",
		"p50_latency_ms", "p95_latency_ms", "average_tokens", "average_cost_usd", "raw_report_uri", "raw_report_sha256",
	} {
		if m[key] == nil || m[key] == "" {
			t.Fatalf("missing RAG evidence field %s", key)
		}
	}
	key := strings.TrimPrefix(m["raw_report_uri"].(string), "artifact://")
	raw, info, err := h.Store.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("read raw report from store: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got, want := m["raw_report_sha256"], hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("raw report hash = %v, want %s", got, want)
	}
	if info.SHA256 != m["raw_report_sha256"] {
		t.Fatalf("store hash = %s, want %v", info.SHA256, m["raw_report_sha256"])
	}
}

func TestStrategyReplayRAGRegressionIsSuccessfulEvidence(t *testing.T) {
	h := NewStrategyReplay(t.TempDir())
	r, err := h.Run(context.Background(), ragReplayPayload(40, true, 0.42, 100))
	if err != nil {
		t.Fatalf("regression must remain a successful worker result: %v", err)
	}
	m := parseResult(t, r)
	if m["result_status"] != "REGRESSION" || m["failed"].(float64) != 100 {
		t.Fatalf("expected RAG REGRESSION with all cases failed, got status=%v failed=%v", m["result_status"], m["failed"])
	}
	if m["guardrail_violations"].(float64) == 0 {
		t.Fatal("expected at least one guardrail violation")
	}
}

func TestStrategyReplayRAGRejectsIncompleteEvidenceInputs(t *testing.T) {
	payload := ragReplayPayload(10, true, 0.42, 100)
	delete(payload, "producer_commit")
	_, err := NewStrategyReplay(t.TempDir()).Run(context.Background(), payload)
	if err == nil {
		t.Fatal("expected missing producer_commit to fail")
	}
	var ee *worker.ExecutionError
	if !errors.As(err, &ee) || ee.Retryable {
		t.Fatalf("expected non-retryable payload error, got %T: %v", err, err)
	}
}
