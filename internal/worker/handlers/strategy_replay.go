package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
)

// StrategyReplay runs a deterministic historical replay of a candidate strategy
// config against N synthetic cases. It is a pure function of the immutable
// execution snapshot carried in the payload, so the same snapshot always yields
// the same result — regardless of which worker executes it, or how many times
// (at-least-once). No wall-clock timing or randomness influences the result.
type StrategyReplay struct{}

// NewStrategyReplay creates a StrategyReplay handler.
func NewStrategyReplay() *StrategyReplay { return &StrategyReplay{} }

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
