package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

const maxGatewayResponseBytes = 64 << 10

// GatewayLLMEval executes a bounded shard through an HTTP gateway. Every
// successful case is persisted before the next one starts, so a lease reclaim
// resumes from durable receipts instead of paying for completed calls again.
type GatewayLLMEval struct {
	artifactDir          string
	store                artifacts.Store
	storeErr             error
	url, profile, apiKey string
	timeout              time.Duration
	maxCases             int
	maxCost              float64
	allowlists           map[string]map[string]bool
	providerGate         chan struct{}
	client               *http.Client
}

func NewGatewayLLMEvalFromEnv(artifactDir string) *GatewayLLMEval {
	store, err := artifacts.NewLocalStore(artifactDir, "", 64<<20)
	h := NewGatewayLLMEvalFromEnvWithStore(store)
	h.artifactDir, h.storeErr = artifactDir, err
	return h
}

func NewGatewayLLMEvalFromEnvWithStore(store artifacts.Store) *GatewayLLMEval {
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
		store: store, url: strings.TrimRight(os.Getenv("EVAL_GATEWAY_URL"), "/"), profile: os.Getenv("EVAL_GATEWAY_PROFILE"), apiKey: os.Getenv("EVAL_GATEWAY_API_KEY"),
		timeout: timeout, maxCases: maxCases, maxCost: maxCost, providerGate: make(chan struct{}, providerConcurrency), client: newGatewayHTTPClient(timeout),
		allowlists: map[string]map[string]bool{
			"dataset_version": parseAllowlist(os.Getenv("EVAL_DATASET_VERSION_ALLOWLIST")), "model_profile": parseAllowlist(os.Getenv("EVAL_MODEL_PROFILE_ALLOWLIST")),
			"prompt_version": parseAllowlist(os.Getenv("EVAL_PROMPT_VERSION_ALLOWLIST")), "retriever_version": parseAllowlist(os.Getenv("EVAL_RETRIEVER_VERSION_ALLOWLIST")),
			"evaluator_schema_version": parseAllowlist(os.Getenv("EVAL_SCHEMA_VERSION_ALLOWLIST")),
		},
	}
}

func newGatewayHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
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
	if h.storeErr != nil {
		return nil, fmt.Errorf("initialize gateway eval artifact store: %w", h.storeErr)
	}
	if h.store == nil {
		return nil, fmt.Errorf("gateway eval artifact store is not configured")
	}
	fingerprint := p.fingerprint()
	baseKey := "eval/gateway/" + fingerprint
	receipts := make(map[int]evalCaseReceipt, p.CaseEnd-p.CaseStart)
	for caseID := p.CaseStart; caseID < p.CaseEnd; caseID++ {
		raw, _, getErr := h.store.Get(ctx, fmt.Sprintf("%s/receipts/%d.json", baseKey, caseID))
		if errors.Is(getErr, artifacts.ErrNotFound) {
			continue
		}
		if getErr != nil {
			return nil, fmt.Errorf("read gateway case receipt %d: %w", caseID, getErr)
		}
		var receipt evalCaseReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil || receipt.CaseID != caseID || receipt.RequestID != p.requestID(caseID) {
			return nil, fmt.Errorf("gateway case receipt %d is corrupt or mismatched: %w", caseID, artifacts.ErrCorrupt)
		}
		receipts[caseID] = receipt
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
		raw, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("marshal gateway case receipt: %w", err)
		}
		if _, err := h.store.PutIfAbsent(ctx, fmt.Sprintf("%s/receipts/%d.json", baseKey, caseID), raw, "application/json"); err != nil {
			return nil, fmt.Errorf("persist gateway case receipt %d: %w", caseID, err)
		}
		receipts[caseID] = r
		if h.maxCost >= 0 && currentCost(receipts) > h.maxCost {
			return nil, worker.NewNonRetryableError(fmt.Errorf("eval cost cap %.6f USD has been exceeded", h.maxCost), domain.ErrorTypeBadPayload)
		}
	}
	if len(receipts) != p.CaseEnd-p.CaseStart {
		return nil, fmt.Errorf("eval receipts incomplete")
	}
	finalKey := baseKey + "/case-results.jsonl"
	var caseResults bytes.Buffer
	for _, receipt := range orderedReceipts(receipts) {
		if err := json.NewEncoder(&caseResults).Encode(receipt); err != nil {
			return nil, fmt.Errorf("encode gateway case results: %w", err)
		}
	}
	if _, err := h.store.PutIfAbsent(ctx, finalKey, caseResults.Bytes(), "application/x-ndjson"); err != nil {
		return nil, fmt.Errorf("persist gateway case results: %w", err)
	}
	passed, failed, in, out, cost, providerWaitMS, providerMS := summarizeReceipts(receipts)
	manifest := map[string]any{"version": "2", "run_id": p.RunID, "fingerprint": fingerprint, "dataset_version": p.DatasetVersion, "dataset_uri": p.DatasetURI, "dataset_sha256": p.DatasetSHA256, "model_profile": p.ModelProfile, "prompt_version": p.PromptVersion, "retriever_version": p.RetrieverVersion, "evaluator_schema_version": p.EvaluatorSchemaVersion, "case_start": p.CaseStart, "case_end": p.CaseEnd, "passed": passed, "failed": failed, "input_tokens": in, "output_tokens": out, "cost_usd": cost, "provider_wait_ms": providerWaitMS, "provider_duration_ms": providerMS}
	manifestKey := baseKey + "/manifest.json"
	if err := putJSONArtifact(ctx, h.store, manifestKey, manifest); err != nil {
		return nil, err
	}
	summary := map[string]any{"passed": passed, "failed": failed, "total_cases": passed + failed, "cost_usd": cost, "provider_wait_ms": providerWaitMS, "provider_duration_ms": providerMS}
	if err := putJSONArtifact(ctx, h.store, baseKey+"/summary.json", summary); err != nil {
		return nil, err
	}
	var failures bytes.Buffer
	for _, receipt := range orderedReceipts(receipts) {
		if receipt.Status != "passed" {
			if err := json.NewEncoder(&failures).Encode(receipt); err != nil {
				return nil, fmt.Errorf("encode gateway failures: %w", err)
			}
		}
	}
	if _, err := h.store.PutIfAbsent(ctx, baseKey+"/failures.jsonl", failures.Bytes(), "application/x-ndjson"); err != nil {
		return nil, fmt.Errorf("persist gateway failures: %w", err)
	}
	return &worker.TaskExecResult{Summary: fmt.Sprintf("gateway LLM eval shard %s [%d,%d): %d/%d passed", p.RunID, p.CaseStart, p.CaseEnd, passed, p.CaseEnd-p.CaseStart), Artifacts: map[string]string{"eval_manifest_uri": "artifact://" + manifestKey, "case_results_uri": "artifact://" + finalKey, "result_fingerprint": fingerprint, "run_id": p.RunID, "dataset_version": p.DatasetVersion, "model_profile": p.ModelProfile, "prompt_version": p.PromptVersion}, Metrics: map[string]float64{"total_cases": float64(p.CaseEnd - p.CaseStart), "passed": float64(passed), "failed": float64(failed), "input_tokens": float64(in), "output_tokens": float64(out), "cost_usd": cost, "provider_wait_ms": providerWaitMS, "provider_duration_ms": providerMS}}, nil
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
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxGatewayResponseBytes+1))
	if err != nil {
		return evalCaseReceipt{}, fmt.Errorf("read gateway response: %w", err)
	}
	if len(responseBody) > maxGatewayResponseBytes {
		return evalCaseReceipt{}, fmt.Errorf("gateway response exceeds %d bytes", maxGatewayResponseBytes)
	}
	var decoded struct {
		Status       string  `json:"status"`
		Score        float64 `json:"score"`
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
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

func putJSONArtifact(ctx context.Context, store artifacts.Store, key string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal artifact %s: %w", key, err)
	}
	if _, err := store.PutIfAbsent(ctx, key, raw, "application/json"); err != nil {
		return fmt.Errorf("persist artifact %s: %w", key, err)
	}
	return nil
}
func currentCost(receipts map[int]evalCaseReceipt) float64 {
	total := 0.0
	for _, r := range orderedReceipts(receipts) {
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
