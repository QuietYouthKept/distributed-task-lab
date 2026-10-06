package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

// GatewayLLMEval executes a bounded shard through an HTTP gateway. Every
// successful case is persisted before the next one starts, so a lease reclaim
// resumes from durable receipts instead of paying for completed calls again.
type GatewayLLMEval struct {
	artifactDir          string
	url, profile, apiKey string
	timeout              time.Duration
	maxCases             int
	maxCost              float64
	allowlists           map[string]map[string]bool
	providerGate         chan struct{}
	client               *http.Client
}

func NewGatewayLLMEvalFromEnv(artifactDir string) *GatewayLLMEval {
	timeout := envEvalDuration("EVAL_REQUEST_TIMEOUT", 30*time.Second)
	maxCases := envEvalInt("EVAL_MAX_CASES_PER_SHARD", 50)
	if maxCases < 1 {
		maxCases = 50
	}
	providerConcurrency := envEvalInt("EVAL_PROVIDER_CONCURRENCY", 8)
	if providerConcurrency < 1 {
		providerConcurrency = 1
	}
	maxCost := envEvalFloat("EVAL_MAX_COST_USD", 10)
	return &GatewayLLMEval{
		artifactDir: artifactDir, url: strings.TrimRight(os.Getenv("EVAL_GATEWAY_URL"), "/"), profile: os.Getenv("EVAL_GATEWAY_PROFILE"), apiKey: os.Getenv("EVAL_GATEWAY_API_KEY"),
		timeout: timeout, maxCases: maxCases, maxCost: maxCost, providerGate: make(chan struct{}, providerConcurrency), client: &http.Client{Timeout: timeout},
		allowlists: map[string]map[string]bool{
			"dataset_version": parseAllowlist(os.Getenv("EVAL_DATASET_VERSION_ALLOWLIST")), "model_profile": parseAllowlist(os.Getenv("EVAL_MODEL_PROFILE_ALLOWLIST")),
			"prompt_version": parseAllowlist(os.Getenv("EVAL_PROMPT_VERSION_ALLOWLIST")), "retriever_version": parseAllowlist(os.Getenv("EVAL_RETRIEVER_VERSION_ALLOWLIST")),
			"evaluator_schema_version": parseAllowlist(os.Getenv("EVAL_SCHEMA_VERSION_ALLOWLIST")),
		},
	}
}

func (h *GatewayLLMEval) Run(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	if h.url == "" || h.profile == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("EVAL_GATEWAY_URL and EVAL_GATEWAY_PROFILE are required for gateway backend"), domain.ErrorTypeBadPayload)
	}
	p, err := parseGatewayEvalPayload(payload, h.maxCases, h.allowlists)
	if err != nil {
		return nil, err
	}
	fingerprint := p.fingerprint()
	dir := filepath.Join(h.artifactDir, "eval", p.RunID, fingerprint)
	finalPath, partialPath := filepath.Join(dir, "case-results.jsonl"), filepath.Join(dir, "case-results.jsonl.partial")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create eval artifact directory: %w", err)
	}
	receipts, err := readEvalReceipts(finalPath)
	if err != nil {
		return nil, err
	}
	if len(receipts) == 0 {
		receipts, err = readEvalReceipts(partialPath)
		if err != nil {
			return nil, err
		}
	}
	if h.maxCost >= 0 && currentCost(receipts) > h.maxCost {
		return nil, worker.NewNonRetryableError(fmt.Errorf("eval cost cap %.6f USD has been exceeded", h.maxCost), domain.ErrorTypeBadPayload)
	}
	for caseID := p.CaseStart; caseID < p.CaseEnd; caseID++ {
		if _, ok := receipts[caseID]; ok {
			continue
		}
		r, err := h.callCase(ctx, p, caseID)
		if err != nil {
			return nil, err
		}
		if err := appendEvalReceipt(partialPath, r); err != nil {
			return nil, err
		}
		receipts[caseID] = r
		if h.maxCost >= 0 && currentCost(receipts) > h.maxCost {
			return nil, worker.NewNonRetryableError(fmt.Errorf("eval cost cap %.6f USD has been exceeded", h.maxCost), domain.ErrorTypeBadPayload)
		}
	}
	if len(receipts) != p.CaseEnd-p.CaseStart {
		return nil, fmt.Errorf("eval receipts incomplete")
	}
	if err := writeEvalReceiptsAtomically(partialPath, finalPath, orderedReceipts(receipts)); err != nil {
		return nil, err
	}
	passed, failed, in, out, cost, providerWaitMS, providerMS := summarizeReceipts(receipts)
	manifest := map[string]any{"version": "2", "run_id": p.RunID, "fingerprint": fingerprint, "dataset_version": p.DatasetVersion, "dataset_uri": p.DatasetURI, "dataset_sha256": p.DatasetSHA256, "model_profile": p.ModelProfile, "prompt_version": p.PromptVersion, "retriever_version": p.RetrieverVersion, "evaluator_schema_version": p.EvaluatorSchemaVersion, "case_start": p.CaseStart, "case_end": p.CaseEnd, "passed": passed, "failed": failed, "input_tokens": in, "output_tokens": out, "cost_usd": cost, "provider_wait_ms": providerWaitMS, "provider_duration_ms": providerMS, "completed_at": nowUTC()}
	if err := writeJSONAtomically(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return nil, err
	}
	summary := map[string]any{"passed": passed, "failed": failed, "total_cases": passed + failed, "cost_usd": cost, "provider_wait_ms": providerWaitMS, "provider_duration_ms": providerMS}
	if err := writeJSONAtomically(filepath.Join(dir, "summary.json"), summary); err != nil {
		return nil, err
	}
	if err := writeFailuresAtomically(filepath.Join(dir, "failures.jsonl"), orderedReceipts(receipts)); err != nil {
		return nil, err
	}
	return &worker.TaskExecResult{Summary: fmt.Sprintf("gateway LLM eval shard %s [%d,%d): %d/%d passed", p.RunID, p.CaseStart, p.CaseEnd, passed, p.CaseEnd-p.CaseStart), Artifacts: map[string]string{"eval_manifest_uri": filepath.Join(dir, "manifest.json"), "case_results_uri": finalPath, "result_fingerprint": fingerprint, "run_id": p.RunID, "dataset_version": p.DatasetVersion, "model_profile": p.ModelProfile, "prompt_version": p.PromptVersion}, Metrics: map[string]float64{"total_cases": float64(p.CaseEnd - p.CaseStart), "passed": float64(passed), "failed": float64(failed), "input_tokens": float64(in), "output_tokens": float64(out), "cost_usd": cost, "provider_wait_ms": providerWaitMS, "provider_duration_ms": providerMS}}, nil
}

func (h *GatewayLLMEval) callCase(ctx context.Context, p gatewayEvalPayload, caseID int) (evalCaseReceipt, error) {
	request := map[string]any{"request_id": p.requestID(caseID), "profile": h.profile, "run_id": p.RunID, "case_id": caseID, "dataset_uri": p.DatasetURI, "dataset_sha256": p.DatasetSHA256, "dataset_version": p.DatasetVersion, "model_profile": p.ModelProfile, "prompt_version": p.PromptVersion, "retriever_version": p.RetrieverVersion, "evaluator_schema_version": p.EvaluatorSchemaVersion}
	raw, _ := json.Marshal(request)
	waitStarted := time.Now()
	select {
	case h.providerGate <- struct{}{}:
		defer func() { <-h.providerGate }()
	case <-ctx.Done():
		return evalCaseReceipt{}, ctx.Err()
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url+"/v1/evaluations", bytes.NewReader(raw))
	if err != nil {
		return evalCaseReceipt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	response, err := h.client.Do(req)
	if err != nil {
		return evalCaseReceipt{}, fmt.Errorf("gateway request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return evalCaseReceipt{}, fmt.Errorf("gateway returned %s", response.Status)
	}
	var decoded struct {
		Status       string  `json:"status"`
		Score        float64 `json:"score"`
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return evalCaseReceipt{}, fmt.Errorf("decode gateway response: %w", err)
	}
	if decoded.Status != "passed" && decoded.Status != "failed" {
		return evalCaseReceipt{}, fmt.Errorf("gateway returned invalid status")
	}
	if decoded.InputTokens < 0 || decoded.OutputTokens < 0 || !finiteNonNegative(decoded.CostUSD) {
		return evalCaseReceipt{}, fmt.Errorf("gateway returned invalid usage")
	}
	return evalCaseReceipt{CaseID: caseID, RequestID: p.requestID(caseID), Status: decoded.Status, Score: decoded.Score, InputTokens: decoded.InputTokens, OutputTokens: decoded.OutputTokens, CostUSD: decoded.CostUSD, ProviderWaitMS: float64(started.Sub(waitStarted)) / float64(time.Millisecond), ProviderMS: float64(time.Since(started)) / float64(time.Millisecond)}, nil
}

func readEvalReceipts(path string) (map[int]evalCaseReceipt, error) {
	receipts := map[int]evalCaseReceipt{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return receipts, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		var r evalCaseReceipt
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("read eval receipt: %w", err)
		}
		receipts[r.CaseID] = r
	}
	return receipts, scanner.Err()
}
func appendEvalReceipt(path string, receipt evalCaseReceipt) error {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
func writeEvalReceiptsAtomically(partial, final string, receipts []evalCaseReceipt) error {
	if _, err := os.Stat(final); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(partial); err == nil {
		return os.Rename(partial, final)
	} else if !os.IsNotExist(err) {
		return err
	}
	var b strings.Builder
	for _, r := range receipts {
		raw, _ := json.Marshal(r)
		b.Write(raw)
		b.WriteByte('\n')
	}
	return writeBytesAtomically(final, []byte(b.String()))
}
func writeFailuresAtomically(path string, receipts []evalCaseReceipt) error {
	var b strings.Builder
	for _, r := range receipts {
		if r.Status != "passed" {
			raw, _ := json.Marshal(r)
			b.Write(raw)
			b.WriteByte('\n')
		}
	}
	return writeBytesAtomically(path, []byte(b.String()))
}
func writeJSONAtomically(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomically(path, raw)
}
func writeBytesAtomically(path string, raw []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func currentCost(receipts map[int]evalCaseReceipt) float64 {
	total := 0.0
	for _, r := range receipts {
		total += r.CostUSD
	}
	return total
}
func envEvalInt(name string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n != 0 {
		return n
	}
	return fallback
}
func envEvalFloat(name string, fallback float64) float64 {
	if n, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil {
		return n
	}
	return fallback
}
func envEvalDuration(name string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(name)); err == nil && d > 0 {
		return d
	}
	return fallback
}
