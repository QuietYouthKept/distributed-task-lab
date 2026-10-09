package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
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
	h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", apiKey: "test-secret", timeout: time.Second, maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 2), client: server.Client()}
	first, err := h.Run(context.Background(), gatewayPayload(0, 3))
	if err != nil {
		t.Fatal(err)
	}
	if first.Metrics["passed"] != 3 || first.Metrics["cost_usd"] != 0.03 {
		t.Fatalf("unexpected metrics %#v", first.Metrics)
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
	manifestKey := strings.TrimPrefix(second.Artifacts["eval_manifest_uri"], "artifact://")
	manifest, _, err := h.store.Get(context.Background(), manifestKey)
	if err != nil {
		t.Fatal(err)
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
	h := &GatewayLLMEval{store: store, url: server.URL, profile: "test", timeout: time.Second, maxCases: 50, maxCost: 1, allowlists: map[string]map[string]bool{}, providerGate: make(chan struct{}, 1), client: server.Client()}
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
