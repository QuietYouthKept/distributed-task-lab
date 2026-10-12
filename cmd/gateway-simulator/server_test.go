package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testEvaluationBody(t *testing.T) []byte {
	t.Helper()
	input := evaluationRequest{
		RequestID: "request-1", Profile: "staging-sim", RunID: "run-1", CaseID: 1,
		DatasetURI: "artifact://datasets/sample.jsonl", DatasetSHA256: strings.Repeat("a", 64),
		DatasetVersion: "dataset-v1", ModelProfile: "simulated-model", PromptVersion: "prompt-v1",
		RetrieverVersion: "retriever-v1", EvaluatorSchemaVersion: "2",
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func postEvaluation(t *testing.T, client *http.Client, url, key string, body []byte) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/v1/evaluations", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

func TestGatewaySimulatorRequiresAuthenticationAndValidContract(t *testing.T) {
	sim, err := newGatewaySimulator("test-only-secret", "normal", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sim.Handler())
	t.Cleanup(server.Close)

	response, err := postEvaluation(t, server.Client(), server.URL, "wrong-secret", testEvaluationBody(t))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", response.StatusCode, http.StatusUnauthorized)
	}
	response, err = postEvaluation(t, server.Client(), server.URL, "test-only-secret", testEvaluationBody(t))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", response.StatusCode, http.StatusOK)
	}
	var decoded evaluationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "passed" || decoded.CostUSD != 0 {
		t.Fatalf("unexpected simulated response: %+v", decoded)
	}
}

func TestGatewaySimulatorDeduplicatesAndRejectsFingerprintMismatch(t *testing.T) {
	sim, err := newGatewaySimulator("test-only-secret", "normal", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sim.Handler())
	t.Cleanup(server.Close)
	body := testEvaluationBody(t)
	for i := 0; i < 2; i++ {
		response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", body)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("replay status=%d want=%d", response.StatusCode, http.StatusOK)
		}
	}
	if got := sim.executionCount(); got != 1 {
		t.Fatalf("executions=%d want=1", got)
	}

	var input evaluationRequest
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	input.CaseID++
	changed, _ := json.Marshal(input)
	response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", changed)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("fingerprint mismatch status=%d want=%d", response.StatusCode, http.StatusConflict)
	}
}

func TestGatewaySimulatorConcurrentIdenticalRequestsExecuteOnce(t *testing.T) {
	sim, err := newGatewaySimulator("test-only-secret", "normal", 20*time.Millisecond, 20)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sim.Handler())
	t.Cleanup(server.Close)
	body := testEvaluationBody(t)
	const callers = 8
	statuses := make(chan int, callers)
	errorsSeen := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", body)
			if err != nil {
				errorsSeen <- err
				return
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	for i := 0; i < callers; i++ {
		select {
		case err := <-errorsSeen:
			t.Fatalf("concurrent idempotent request failed: %v", err)
		case status := <-statuses:
			if status != http.StatusOK {
				t.Fatalf("concurrent replay status=%d want=%d", status, http.StatusOK)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent idempotent requests did not complete within bound")
		}
	}
	if got := sim.executionCount(); got != 1 {
		t.Fatalf("concurrent identical requests executed %d times, want 1", got)
	}
}

func TestGatewaySimulatorRateLimitAndTimeoutModes(t *testing.T) {
	t.Run("429", func(t *testing.T) {
		sim, err := newGatewaySimulator("test-only-secret", "rate_limit", 0, 20)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(sim.Handler())
		t.Cleanup(server.Close)
		response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", testEvaluationBody(t))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") != "1" {
			t.Fatalf("status=%d retry-after=%q", response.StatusCode, response.Header.Get("Retry-After"))
		}
	})
	t.Run("timeout", func(t *testing.T) {
		sim, err := newGatewaySimulator("test-only-secret", "timeout", 60*time.Millisecond, 20)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(sim.Handler())
		t.Cleanup(server.Close)
		client := &http.Client{Timeout: 10 * time.Millisecond}
		response, err := postEvaluation(t, client, server.URL, "test-only-secret", testEvaluationBody(t))
		if response != nil {
			response.Body.Close()
		}
		if err == nil {
			t.Fatal("client deadline should observe simulated gateway timeout")
		}
		if got := sim.executionCount(); got != 1 {
			t.Fatalf("timeout should preserve the simulated unknown outcome, executions=%d", got)
		}
	})
}

func TestGatewaySimulatorResponseLossCanBeReplayedIdempotently(t *testing.T) {
	sim, err := newGatewaySimulator("test-only-secret", "response_loss", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sim.Handler())
	t.Cleanup(server.Close)
	body := testEvaluationBody(t)
	if response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", body); err == nil {
		response.Body.Close()
		t.Fatal("first response should be lost")
	}
	response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", body)
	if err != nil {
		t.Fatalf("idempotent replay should return the stored result: %v", err)
	}
	response.Body.Close()
	if got := sim.executionCount(); response.StatusCode != http.StatusOK || got != 1 {
		t.Fatalf("replay status=%d executions=%d", response.StatusCode, got)
	}
}

func TestGatewaySimulatorRestartLosesInMemoryIdempotencyState(t *testing.T) {
	body := testEvaluationBody(t)
	first, err := newGatewaySimulator("test-only-secret", "normal", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	firstServer := httptest.NewServer(first.Handler())
	response, err := postEvaluation(t, firstServer.Client(), firstServer.URL, "test-only-secret", body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	firstServer.Close()

	// A new process starts with an empty in-memory ledger. This test documents
	// the fixture's limitation; it does not claim durable provider idempotency.
	restarted, err := newGatewaySimulator("test-only-secret", "normal", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	secondServer := httptest.NewServer(restarted.Handler())
	t.Cleanup(secondServer.Close)
	response, err = postEvaluation(t, secondServer.Client(), secondServer.URL, "test-only-secret", body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || restarted.executionCount() != 1 {
		t.Fatalf("restart replay status=%d executions=%d, want 200 and 1 simulated execution", response.StatusCode, restarted.executionCount())
	}
}

func TestGatewaySimulatorFaultModesAreDeterministic(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		statusCode int
		wantStatus string
		wantTokens int
		wantCost   float64
		wantDecode bool
	}{
		{name: "explicit failure", mode: "explicit_failure", statusCode: http.StatusOK, wantStatus: "failed", wantTokens: 17, wantDecode: true},
		{name: "HTTP 500", mode: "http_500", statusCode: http.StatusInternalServerError},
		{name: "malformed response", mode: "malformed_response", statusCode: http.StatusOK},
		{name: "invalid usage", mode: "invalid_usage", statusCode: http.StatusOK, wantStatus: "passed", wantTokens: -1, wantDecode: true},
		{name: "cost exceeded", mode: "cost_exceeded", statusCode: http.StatusOK, wantStatus: "passed", wantTokens: 17, wantCost: 1_000_000, wantDecode: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sim, err := newGatewaySimulator("test-only-secret", test.mode, 0, 20)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(sim.Handler())
			t.Cleanup(server.Close)
			response, err := postEvaluation(t, server.Client(), server.URL, "test-only-secret", testEvaluationBody(t))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.statusCode {
				t.Fatalf("status=%d want=%d", response.StatusCode, test.statusCode)
			}
			if !test.wantDecode {
				if test.mode == "malformed_response" {
					if err := json.NewDecoder(response.Body).Decode(&evaluationResponse{}); err == nil {
						t.Fatal("malformed response unexpectedly decoded")
					}
				}
				return
			}
			var decoded evaluationResponse
			if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Status != test.wantStatus || decoded.InputTokens != test.wantTokens || decoded.CostUSD != test.wantCost {
				t.Fatalf("response=%+v want status=%q tokens=%d cost=%v", decoded, test.wantStatus, test.wantTokens, test.wantCost)
			}
		})
	}
}
