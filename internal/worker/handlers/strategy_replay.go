package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

const (
	replayJobSchemaVersion = "1"
	replaySimulatorVersion = "go-v1"
	replayBaselineRatio    = 0.15
	replayMinSensitivity   = 0.05
	replayMaxSensitivity   = 0.35
	replayDefaultThreshold = 0.02
	ragStrategyKey         = "rag_retrieval_policy"
)

// StrategyReplay runs a deterministic historical replay of a candidate strategy
// config against N synthetic cases. It is a pure function of the immutable
// execution snapshot carried in the payload, so the same snapshot always yields
// the same result — regardless of which worker executes it, or how many times
// (at-least-once). No wall-clock timing or randomness influences the result.
type StrategyReplay struct{ ArtifactDir string }

// NewStrategyReplay creates a StrategyReplay handler.
func NewStrategyReplay(artifactDirs ...string) *StrategyReplay {
	dir := "artifacts/strategy-replay"
	if len(artifactDirs) > 0 && artifactDirs[0] != "" {
		dir = artifactDirs[0]
	}
	return &StrategyReplay{ArtifactDir: dir}
}

// Run executes the deterministic replay. A regression is a SUCCESSFUL business
// result (result_status = REGRESSION), never a system failure; only malformed or
// invalid payloads are returned as (non-retryable) errors.
func (h *StrategyReplay) Run(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	startedAt := time.Now().UTC()

	proposalID := getString(payload, "proposal_id", "")
	if proposalID == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("proposal_id is required"), domain.ErrorTypeBadPayload)
	}
	if getString(payload, "job_schema_version", "") != replayJobSchemaVersion {
		return nil, worker.NewNonRetryableError(fmt.Errorf("unsupported job_schema_version"), domain.ErrorTypeBadPayload)
	}
	resultSchemaVersion := getString(payload, "replay_result_schema_version", "")
	if resultSchemaVersion != replayJobSchemaVersion {
		return nil, worker.NewNonRetryableError(fmt.Errorf("unsupported replay_result_schema_version"), domain.ErrorTypeBadPayload)
	}
	if getString(payload, "simulator_version", "") != replaySimulatorVersion {
		return nil, worker.NewNonRetryableError(fmt.Errorf("unsupported simulator_version"), domain.ErrorTypeBadPayload)
	}
	executionFingerprint := getString(payload, "execution_fingerprint", "")
	if executionFingerprint == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("execution_fingerprint is required"), domain.ErrorTypeBadPayload)
	}

	candidateCfg := getMap(payload, "candidate_config")
	if candidateCfg == nil {
		return nil, worker.NewNonRetryableError(fmt.Errorf("candidate_config is required"), domain.ErrorTypeBadPayload)
	}
	if getString(payload, "strategy_key", "") == ragStrategyKey {
		return h.runRAG(ctx, payload, candidateCfg, startedAt)
	}
	if getString(payload, "strategy_key", "") != "new_user_feed_exploration" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("unsupported strategy_key"), domain.ErrorTypeBadPayload)
	}
	candidateRatio, ok := candidateCfg["exploration_ratio"].(float64)
	if !ok {
		return nil, worker.NewNonRetryableError(fmt.Errorf("candidate_config.exploration_ratio must be a number"), domain.ErrorTypeBadPayload)
	}

	caseCount := getInt(payload, "case_count", 50)
	if caseCount <= 0 {
		return nil, worker.NewNonRetryableError(fmt.Errorf("case_count must be > 0"), domain.ErrorTypeBadPayload)
	}

	threshold := replayDefaultThreshold
	if guardrailCfg := getMap(payload, "guardrail_config"); guardrailCfg != nil {
		if v, ok := guardrailCfg["negative_feedback_delta"].(float64); ok && v >= 0 {
			threshold = v
		}
	}

	seed := int64(getInt(payload, "seed", 42))

	// Deterministic replay loop over N synthetic historical cases.
	delta := candidateRatio - replayBaselineRatio
	passed, failed, violations := 0, 0, 0
	failures := []map[string]any{}
	for i := 0; i < caseCount; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sensitivity := replayMinSensitivity
		if caseCount > 1 {
			sensitivity = replayMinSensitivity +
				(replayMaxSensitivity-replayMinSensitivity)*float64(i)/float64(caseCount-1)
		}
		negFbDelta := sensitivity * delta

		if negFbDelta > threshold {
			failed++
			violations++
			if len(failures) < 3 {
				failures = append(failures, map[string]any{
					"case_index": i,
					"metric":     "negative_feedback_rate",
					"observed":   round6(negFbDelta),
					"threshold":  threshold,
				})
			}
		} else {
			passed++
		}
	}

	risk := "LOW"
	if failed > 0 {
		if failed*5 <= caseCount {
			risk = "MEDIUM"
		} else {
			risk = "HIGH"
		}
	}

	resultStatus := "PASS"
	if failed > 0 {
		resultStatus = "REGRESSION"
	}

	result := map[string]any{
		"job_schema_version":           replayJobSchemaVersion,
		"replay_result_schema_version": resultSchemaVersion,
		"proposal_id":                  proposalID,
		"proposal_version":             getInt(payload, "proposal_version", 1),
		"strategy_key":                 getString(payload, "strategy_key", ""),
		"scenario_set":                 getString(payload, "scenario_set", "default"),
		"scenario_set_version":         getString(payload, "scenario_set_version", "v1"),
		"simulator_version":            replaySimulatorVersion,
		"policy_version":               getString(payload, "policy_version", "v1"),
		"seed":                         seed,
		"case_count":                   caseCount,
		"execution_fingerprint":        executionFingerprint,
		"total_cases":                  caseCount,
		"passed":                       passed,
		"failed":                       failed,
		"guardrail_violations":         violations,
		"risk_summary":                 risk,
		"result_status":                resultStatus,
		"representative_failures":      failures,
		"started_at":                   startedAt.Format(time.RFC3339Nano),
		"completed_at":                 time.Now().UTC().Format(time.RFC3339Nano),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("strategy_replay: marshal result: %w", err)
	}

	summary := fmt.Sprintf("%s: %d/%d cases passed, %d guardrail violations", resultStatus, passed, caseCount, violations)
	return &worker.TaskExecResult{
		Summary: summary,
		Artifacts: map[string]string{
			"replay_result":     string(resultJSON),
			"result_status":     resultStatus,
			"risk_summary":      risk,
			"proposal_id":       proposalID,
			"simulator_version": replaySimulatorVersion,
		},
		Metrics: map[string]float64{
			"total_cases":          float64(caseCount),
			"passed":               float64(passed),
			"failed":               float64(failed),
			"guardrail_violations": float64(violations),
		},
	}, nil
}

// runRAG writes a complete, content-addressed offline-evaluation report. It is
// an independent worker receipt: StrategyOps only supplies immutable inputs and
// validates the echoed result; it never fabricates these metrics itself.
//
// The scoring function is deterministic development scaffolding until a real
// retriever/evaluator is wired in. Its artifact shape, provenance checks and
// idempotent content addressing are production-facing contract behavior.
func (h *StrategyReplay) runRAG(ctx context.Context, payload, candidate map[string]any, startedAt time.Time) (*worker.TaskExecResult, error) {
	caseCount := getInt(payload, "case_count", 0)
	if caseCount < 100 {
		return nil, worker.NewNonRetryableError(fmt.Errorf("RAG replay requires case_count >= 100"), domain.ErrorTypeBadPayload)
	}
	topK, ok := candidate["top_k"].(float64)
	if !ok || math.Trunc(topK) != topK || topK < 1 || topK > 50 {
		return nil, worker.NewNonRetryableError(fmt.Errorf("candidate_config.top_k must be an integer in [1, 50]"), domain.ErrorTypeBadPayload)
	}
	reranker, ok := candidate["reranker_enabled"].(bool)
	if !ok {
		return nil, worker.NewNonRetryableError(fmt.Errorf("candidate_config.reranker_enabled must be boolean"), domain.ErrorTypeBadPayload)
	}
	minScore, ok := candidate["min_score"].(float64)
	if !ok || !isFinite(minScore) || minScore < 0 || minScore > 1 {
		return nil, worker.NewNonRetryableError(fmt.Errorf("candidate_config.min_score must be finite and in [0, 1]"), domain.ErrorTypeBadPayload)
	}

	for _, field := range []string{"dataset_version", "prompt_version", "retriever_version", "evaluator_schema_version"} {
		if getString(payload, field, "") == "" {
			return nil, worker.NewNonRetryableError(fmt.Errorf("%s is required", field), domain.ErrorTypeBadPayload)
		}
	}
	producerCommit := getString(payload, "producer_commit", "")
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(producerCommit) {
		return nil, worker.NewNonRetryableError(fmt.Errorf("producer_commit must be a 40-character SHA"), domain.ErrorTypeBadPayload)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	recall := math.Min(0.99, 0.68+0.012*topK+boolFloat(reranker)*0.035)
	mrr := math.Min(0.99, 0.42+0.009*topK+boolFloat(reranker)*0.045)
	ndcg := math.Min(0.99, 0.48+0.009*topK+boolFloat(reranker)*0.04)
	answerRecall := math.Min(0.99, 0.62+0.011*topK+boolFloat(reranker)*0.05)
	unsupported := math.Max(0.001, 0.021-boolFloat(reranker)*0.006-minScore*0.01)
	refusal := math.Min(0.99, 0.82+boolFloat(reranker)*0.04)
	p50 := 70.0 + 2.0*topK + boolFloat(reranker)*12.0
	p95 := 115.0 + 3.0*topK + boolFloat(reranker)*22.0
	tokens := 360.0 + 18.0*topK + boolFloat(reranker)*100.0
	cost := 0.0012 + 0.00002*topK + boolFloat(reranker)*0.00018

	violations := 0
	if unsupported-0.012 > ragGuardrailThreshold(payload, "unsupported_claim_rate", 0) {
		violations++
	}
	if (p95-150.0)/150.0 > ragGuardrailThreshold(payload, "p95_latency_ms", 0.20) {
		violations++
	}
	if (cost-0.0015)/0.0015 > ragGuardrailThreshold(payload, "average_cost_usd", 0.25) {
		violations++
	}
	failed := 0
	if violations > 0 {
		failed = caseCount
	}
	passed := caseCount - failed
	resultStatus, risk := "PASS", "LOW"
	if failed > 0 {
		resultStatus, risk = "REGRESSION", "HIGH"
	}

	report := map[string]any{
		"dataset_version": getString(payload, "dataset_version", ""), "prompt_version": getString(payload, "prompt_version", ""),
		"retriever_version": getString(payload, "retriever_version", ""), "producer_commit": producerCommit,
		"evaluator_schema_version": getString(payload, "evaluator_schema_version", ""), "case_count": caseCount,
		"recall_at_k": round6(recall), "mrr": round6(mrr), "ndcg": round6(ndcg), "answer_point_recall": round6(answerRecall),
		"unsupported_claim_rate": round6(unsupported), "refusal_accuracy": round6(refusal), "p50_latency_ms": round6(p50),
		"p95_latency_ms": round6(p95), "average_tokens": round6(tokens), "average_cost_usd": round6(cost),
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("rag strategy replay: marshal report: %w", err)
	}
	sum := sha256.Sum256(reportBytes)
	reportHash := hex.EncodeToString(sum[:])
	path := filepath.Join(h.ArtifactDir, "rag-replay", reportHash+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("rag strategy replay: create artifact directory: %w", err)
	}
	if err := os.WriteFile(path, reportBytes, 0o644); err != nil {
		return nil, fmt.Errorf("rag strategy replay: write report: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("rag strategy replay: resolve report path: %w", err)
	}

	result := map[string]any{
		"job_schema_version": replayJobSchemaVersion, "replay_result_schema_version": getString(payload, "replay_result_schema_version", ""),
		"proposal_id": getString(payload, "proposal_id", ""), "proposal_version": getInt(payload, "proposal_version", 1),
		"strategy_key": ragStrategyKey, "scenario_set": getString(payload, "scenario_set", "default"),
		"scenario_set_version": getString(payload, "scenario_set_version", "v1"), "simulator_version": replaySimulatorVersion,
		"policy_version": getString(payload, "policy_version", "v1"), "seed": getInt(payload, "seed", 42), "case_count": caseCount,
		"execution_fingerprint": getString(payload, "execution_fingerprint", ""), "total_cases": caseCount, "passed": passed, "failed": failed,
		"guardrail_violations": violations, "risk_summary": risk, "result_status": resultStatus, "representative_failures": []map[string]any{},
		"started_at": startedAt.Format(time.RFC3339Nano), "completed_at": time.Now().UTC().Format(time.RFC3339Nano),
		"dataset_version": getString(payload, "dataset_version", ""), "prompt_version": getString(payload, "prompt_version", ""),
		"retriever_version": getString(payload, "retriever_version", ""), "producer_commit": producerCommit,
		"evaluator_schema_version": getString(payload, "evaluator_schema_version", ""), "recall_at_k": round6(recall), "mrr": round6(mrr),
		"ndcg": round6(ndcg), "answer_point_recall": round6(answerRecall), "unsupported_claim_rate": round6(unsupported),
		"refusal_accuracy": round6(refusal), "p50_latency_ms": round6(p50), "p95_latency_ms": round6(p95),
		"average_tokens": round6(tokens), "average_cost_usd": round6(cost), "raw_report_uri": "file://" + filepath.ToSlash(absPath),
		"raw_report_sha256": reportHash,
	}
	if failed > 0 {
		result["representative_failures"] = []map[string]any{{"metric": "rag_guardrails", "observed": float64(violations), "threshold": 0}}
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("rag strategy replay: marshal result: %w", err)
	}
	return &worker.TaskExecResult{
		Summary:   fmt.Sprintf("RAG %s: %d/%d cases passed, %d guardrail violations", resultStatus, passed, caseCount, violations),
		Artifacts: map[string]string{"replay_result": string(resultJSON), "raw_report_uri": result["raw_report_uri"].(string), "raw_report_sha256": reportHash},
		Metrics:   map[string]float64{"total_cases": float64(caseCount), "passed": float64(passed), "failed": float64(failed), "guardrail_violations": float64(violations), "average_cost_usd": cost, "p95_latency_ms": p95},
	}, nil
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func isFinite(value float64) bool { return !math.IsInf(value, 0) && !math.IsNaN(value) }

func ragGuardrailThreshold(payload map[string]any, metric string, fallback float64) float64 {
	entry := getMap(getMap(payload, "guardrail_config"), metric)
	if entry == nil {
		return fallback
	}
	if threshold, ok := entry["threshold"].(float64); ok && isFinite(threshold) {
		return threshold
	}
	return fallback
}

func getMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return nil
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
