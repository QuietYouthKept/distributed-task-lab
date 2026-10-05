package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

// LLMEval is a deterministic stand-in for a versioned batch LLM evaluation.
// Its artifact key is derived only from the immutable execution input, so an
// at-least-once retry returns the original shard result rather than producing a
// second final result. A real model gateway can replace evaluateCase while
// preserving this artifact and reconciliation contract.
type LLMEval struct{ ArtifactDir string }

func NewLLMEval(artifactDir string) *LLMEval { return &LLMEval{ArtifactDir: artifactDir} }

func (h *LLMEval) Run(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	datasetVersion := getString(payload, "dataset_version", "")
	modelProfile := getString(payload, "model_profile", "")
	promptVersion := getString(payload, "prompt_version", "")
	runID := getString(payload, "run_id", "")
	start, end := getInt(payload, "case_start", -1), getInt(payload, "case_end", -1)
	if datasetVersion == "" || modelProfile == "" || promptVersion == "" || runID == "" || start < 0 || end <= start {
		return nil, worker.NewNonRetryableError(fmt.Errorf("dataset_version, model_profile, prompt_version, run_id and a non-empty [case_start, case_end) are required"), domain.ErrorTypeBadPayload)
	}

	identity := fmt.Sprintf("%s|%s|%s|%s|%d|%d", datasetVersion, modelProfile, promptVersion, runID, start, end)
	sum := sha256.Sum256([]byte(identity))
	fingerprint := hex.EncodeToString(sum[:])
	path := filepath.Join(h.ArtifactDir, "eval", runID, fingerprint+".json")
	reused := false
	result := evalShardResult{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("read existing eval shard: %w", err)
		}
		reused = true
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read eval shard: %w", err)
	} else {
		result = buildEvalShard(datasetVersion, modelProfile, promptVersion, runID, start, end, fingerprint)
		raw, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal eval shard: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create eval artifact directory: %w", err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return nil, fmt.Errorf("write eval shard: %w", err)
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("LLM eval shard %s [%d,%d): %d/%d passed (reused=%t)", runID, start, end, result.Passed, end-start, reused),
		Artifacts: map[string]string{
			"case_results_uri": path, "result_fingerprint": result.Fingerprint,
			"run_id": runID, "dataset_version": datasetVersion, "model_profile": modelProfile,
			"prompt_version": promptVersion, "idempotent_reused": fmt.Sprintf("%t", reused),
		},
		Metrics: map[string]float64{
			"total_cases": float64(end - start), "passed": float64(result.Passed), "failed": float64(result.Failed),
			"input_tokens": float64(result.InputTokens), "output_tokens": float64(result.OutputTokens), "cost_usd": result.CostUSD,
		},
	}, nil
}

type evalShardResult struct {
	DatasetVersion, ModelProfile, PromptVersion, RunID, Fingerprint string
	CaseStart, CaseEnd, Passed, Failed, InputTokens, OutputTokens   int
	CostUSD                                                         float64
}

func buildEvalShard(dataset, model, prompt, runID string, start, end int, fingerprint string) evalShardResult {
	r := evalShardResult{DatasetVersion: dataset, ModelProfile: model, PromptVersion: prompt, RunID: runID, Fingerprint: fingerprint, CaseStart: start, CaseEnd: end}
	for i := start; i < end; i++ {
		// A stable pseudo-result makes the example fully reproducible without a
		// provider account. It is deliberately not presented as a model score.
		if int(fingerprint[(i*7)%len(fingerprint)])%17 == 0 {
			r.Failed++
		} else {
			r.Passed++
		}
		r.InputTokens += 120 + i%31
		r.OutputTokens += 40 + i%13
	}
	r.CostUSD = float64(r.InputTokens)*0.0000005 + float64(r.OutputTokens)*0.0000015
	return r
}
