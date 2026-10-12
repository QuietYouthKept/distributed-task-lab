package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
)

func gatewayPayload(start, end int) map[string]any {
	return map[string]any{
		"version": "2", "run_id": "release-gate-2026", "dataset_version": "serviceops-v1.2.0-generation-gold",
		"dataset_uri": "artifact://datasets/serviceops.jsonl", "dataset_sha256": strings.Repeat("a", 64),
		"case_start": start, "case_end": end, "model_profile": "qwen2.5-3b-local", "prompt_version": "serviceops-rag-v4",
		"retriever_version": "hybrid-reranker-v2", "evaluator_schema_version": "2",
	}
}

func TestGatewayLLMEvalWritesReceiptsAndReusesFinalArtifact(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("authorization = %q", got)
		}
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		if !strings.Contains(request["request_id"].(string), "release-gate-2026:") {
			t.Errorf("unstable request id %v", request["request_id"])
		}
		mu.Lock()
		calls++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "passed", "score": 0.9, "input_tokens": 10, "output_tokens": 3, "cost_usd": 0.01})
	}))
	defer server.Close()
	store, err := artifacts.NewLocalStore(t.TempDir(), "worker-a", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", apiKey: "test-secret", timeout: time.Second, maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 2), client: newGatewayHTTPClient(time.Second)}
	first, err := h.Run(context.Background(), gatewayPayload(0, 3))
	if err != nil {
		t.Fatal(err)
	}
	if first.Metrics["passed"] != 3 || first.Metrics["cost_usd"] != 0.03 {
		t.Fatalf("unexpected metrics %#v", first.Metrics)
	}
	manifestKey := strings.TrimPrefix(first.Artifacts["eval_manifest_uri"], "artifact://")
	firstManifest, _, err := store.Get(context.Background(), manifestKey)
	if err != nil {
		t.Fatal(err)
	}
	secondWorker := *h
	second, err := secondWorker.Run(context.Background(), gatewayPayload(0, 3))
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 3 {
		t.Fatalf("gateway calls = %d, want 3 after reuse", gotCalls)
	}
	secondManifestKey := strings.TrimPrefix(second.Artifacts["eval_manifest_uri"], "artifact://")
	manifest, _, err := h.store.Get(context.Background(), secondManifestKey)
	if err != nil {
		t.Fatal(err)
	}
	if manifestKey != secondManifestKey || !bytes.Equal(firstManifest, manifest) {
		t.Fatal("replaying persisted receipts changed the immutable manifest")
	}
	if strings.Contains(string(manifest), "test-secret") {
		t.Fatal("gateway API key leaked into artifact")
	}
	for _, name := range []string{"manifest.json", "case-results.jsonl", "summary.json", "failures.jsonl"} {
		if _, err := h.store.Head(context.Background(), strings.TrimSuffix(manifestKey, "/manifest.json")+"/"+name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestGatewayLLMEvalResumesPartialReceipts(t *testing.T) {
	var mu sync.Mutex
	calls := map[int]int{}
	failCaseOne := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			CaseID int `json:"case_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		calls[request.CaseID]++
		shouldFail := request.CaseID == 1 && failCaseOne
		mu.Unlock()
		if shouldFail {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "passed", "input_tokens": 1, "output_tokens": 1, "cost_usd": 0.001})
	}))
	defer server.Close()
	store, err := artifacts.NewLocalStore(t.TempDir(), "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", timeout: time.Second, maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: newGatewayHTTPClient(time.Second)}
	if _, err := h.Run(context.Background(), gatewayPayload(0, 3)); err == nil {
		t.Fatal("expected transient gateway failure")
	}
	mu.Lock()
	failCaseOne = false
	mu.Unlock()
	if _, err := h.Run(context.Background(), gatewayPayload(0, 3)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls[0] != 1 || calls[1] != 2 || calls[2] != 1 {
		t.Fatalf("partial-resume calls = %#v", calls)
	}
}

func TestGatewayLLMEvalRejectsInvalidV2Payload(t *testing.T) {
	h := &GatewayLLMEval{url: "http://example.invalid", profile: "test", maxCases: 50, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: &http.Client{}}
	p := gatewayPayload(0, 51)
	if _, err := h.Run(context.Background(), p); err == nil {
		t.Fatal("expected shard size validation error")
	}
}

func TestGatewayLLMEvalRejectsMalformedAndInvalidUsageResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"status":`},
		{name: "trailing JSON value", body: `{"status":"passed","input_tokens":0,"output_tokens":0,"cost_usd":0} {}`},
		{name: "negative input usage", body: `{"status":"passed","input_tokens":-1,"output_tokens":0,"cost_usd":0}`},
		{name: "negative cost", body: `{"status":"passed","input_tokens":0,"output_tokens":0,"cost_usd":-0.01}`},
		{name: "invalid status", body: `{"status":"unknown","input_tokens":0,"output_tokens":0,"cost_usd":0}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			store, err := artifacts.NewLocalStore(t.TempDir(), "", 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", timeout: time.Second, maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: newGatewayHTTPClient(time.Second)}
			if _, err := h.Run(context.Background(), gatewayPayload(0, 1)); err == nil {
				t.Fatal("invalid gateway response was accepted")
			}
		})
	}
}

func TestGatewayLLMEvalRecordsExplicitCaseFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "failed", "score": 0, "input_tokens": 1, "output_tokens": 2, "cost_usd": 0})
	}))
	defer server.Close()
	store, err := artifacts.NewLocalStore(t.TempDir(), "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", timeout: time.Second, maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: newGatewayHTTPClient(time.Second)}
	result, err := h.Run(context.Background(), gatewayPayload(0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics["failed"] != 1 || result.Metrics["passed"] != 0 {
		t.Fatalf("explicit failure metrics = %#v", result.Metrics)
	}
}

func TestGatewayLLMEvalDoesNotFollowRedirects(t *testing.T) {
	var redirectedRequests int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests++
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "passed", "input_tokens": 1, "output_tokens": 1, "cost_usd": 0})
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	store, err := artifacts.NewLocalStore(t.TempDir(), "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := &GatewayLLMEval{store: store, url: redirector.URL, profile: "test", apiKey: "sensitive-test-key", timeout: time.Second,
		maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: newGatewayHTTPClient(time.Second)}
	if _, err := h.Run(context.Background(), gatewayPayload(0, 1)); err == nil {
		t.Fatal("gateway redirect was accepted")
	}
	if redirectedRequests != 0 {
		t.Fatalf("redirect target received %d request(s), want 0", redirectedRequests)
	}
}

func TestGatewayLLMEvalBoundsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxGatewayResponseBytes+1)))
	}))
	defer server.Close()
	store, err := artifacts.NewLocalStore(t.TempDir(), "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", timeout: time.Second,
		maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: newGatewayHTTPClient(time.Second)}
	if _, err := h.Run(context.Background(), gatewayPayload(0, 1)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized gateway response error=%v, want bounded response rejection", err)
	}
}

func TestStagingSimulationCompositeExecutesOnlyGatewayCandidate(t *testing.T) {
	t.Setenv("EVAL_BACKEND", "gateway")
	t.Setenv("EVAL_GATEWAY_PROFILE", "staging-sim")
	t.Setenv("EVAL_GATEWAY_API_KEY", "fixture-only-secret")
	t.Setenv("EVAL_REQUEST_TIMEOUT", "1s")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/evaluations" || r.Method != http.MethodPost {
			t.Errorf("unexpected gateway request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fixture-only-secret" {
			t.Errorf("authorization leaked or mismatched: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "passed", "score": 1, "input_tokens": 2, "output_tokens": 1, "cost_usd": 0})
	}))
	t.Cleanup(server.Close)
	t.Setenv("EVAL_GATEWAY_URL", server.URL)
	store, err := artifacts.NewLocalStore(t.TempDir(), "worker", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	composite := NewCompositeWithArtifactStore(t.TempDir(), t.TempDir(), taskpayload.StagingSimulationExecutionPolicy(), store)
	payload := gatewayPayload(0, 1)
	payload["execution_mode"] = "gateway"
	result, err := composite.Handle(context.Background(), "eval", payload, 1)
	if err != nil {
		t.Fatalf("approved local gateway candidate failed: %v", err)
	}
	if result == nil || result.Metrics["passed"] != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := composite.Handle(context.Background(), "eval", map[string]any{
		"execution_mode": "deterministic", "dataset_version": "dataset-v1", "model_profile": "model-1",
		"prompt_version": "prompt-1", "run_id": "run-1", "case_start": 0, "case_end": 1,
	}, 1); err == nil || !strings.Contains(err.Error(), "not enabled in this worker deployment") {
		t.Fatalf("unapproved eval mode bypassed worker policy: %v", err)
	}
}
