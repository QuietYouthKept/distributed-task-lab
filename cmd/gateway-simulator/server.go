package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxRequestBytes = 16 << 10

var digestPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type simulatorMode string

const (
	modeNormal       simulatorMode = "normal"
	modeFailure      simulatorMode = "explicit_failure"
	modeRateLimit    simulatorMode = "rate_limit"
	modeServerError  simulatorMode = "http_500"
	modeTimeout      simulatorMode = "timeout"
	modeResponseLoss simulatorMode = "response_loss"
	modeMalformed    simulatorMode = "malformed_response"
	modeInvalidUsage simulatorMode = "invalid_usage"
	modeCostExceeded simulatorMode = "cost_exceeded"
)

type evaluationRequest struct {
	RequestID              string `json:"request_id"`
	Profile                string `json:"profile"`
	RunID                  string `json:"run_id"`
	CaseID                 int    `json:"case_id"`
	DatasetURI             string `json:"dataset_uri"`
	DatasetSHA256          string `json:"dataset_sha256"`
	DatasetVersion         string `json:"dataset_version"`
	ModelProfile           string `json:"model_profile"`
	PromptVersion          string `json:"prompt_version"`
	RetrieverVersion       string `json:"retriever_version"`
	EvaluatorSchemaVersion string `json:"evaluator_schema_version"`
}

type evaluationResponse struct {
	Status       string  `json:"status"`
	Score        float64 `json:"score"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type idempotencyEntry struct {
	fingerprint string
	response    []byte
	done        chan struct{}
}

type gatewaySimulator struct {
	apiKey string
	mode   simulatorMode
	delay  time.Duration
	limit  int

	mu        sync.Mutex
	requests  map[string]*idempotencyEntry
	execution int
}

func newGatewaySimulator(apiKey, mode string, delay time.Duration, limit int) (*gatewaySimulator, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("gateway simulator API key is required")
	}
	selected := simulatorMode(mode)
	if selected == "" {
		selected = modeNormal
	}
	switch selected {
	case modeNormal, modeFailure, modeRateLimit, modeServerError, modeTimeout, modeResponseLoss,
		modeMalformed, modeInvalidUsage, modeCostExceeded:
	default:
		return nil, fmt.Errorf("unsupported gateway simulator mode %q", selected)
	}
	if delay < 0 || delay > 10*time.Second {
		return nil, errors.New("gateway simulator delay must be between 0 and 10s")
	}
	if limit <= 0 || limit > 10000 {
		return nil, errors.New("gateway simulator idempotency limit must be between 1 and 10000")
	}
	return &gatewaySimulator{apiKey: apiKey, mode: selected, delay: delay, limit: limit, requests: make(map[string]*idempotencyEntry)}, nil
}

func (s *gatewaySimulator) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /v1/evaluations", s.evaluate)
	return mux
}

func (s *gatewaySimulator) evaluate(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.apiKey)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var input evaluationRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || !validEvaluationRequest(input) {
		http.Error(w, "invalid evaluation request", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		http.Error(w, "invalid evaluation request", http.StatusBadRequest)
		return
	}
	fingerprintBytes := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])

	if s.mode == modeRateLimit {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "simulated rate limit", http.StatusTooManyRequests)
		return
	}
	if s.mode == modeServerError {
		http.Error(w, "simulated upstream failure", http.StatusInternalServerError)
		return
	}

	entry, replay, conflict := s.begin(input.RequestID, fingerprint)
	if conflict {
		http.Error(w, "request_id reused with different request", http.StatusConflict)
		return
	}
	if replay {
		writeJSON(w, http.StatusOK, entry.response)
		return
	}

	response := evaluationResponse{Status: "passed", Score: 1, InputTokens: 16 + input.CaseID%7, OutputTokens: 8 + input.CaseID%5, CostUSD: 0}
	switch s.mode {
	case modeFailure:
		response.Status = "failed"
		response.Score = 0
	case modeMalformed:
		s.finish(entry, []byte(`{"status":`))
		writeJSON(w, http.StatusOK, entry.response)
		return
	case modeInvalidUsage:
		response.InputTokens = -1
	case modeCostExceeded:
		response.CostUSD = 1_000_000
	}
	encoded, _ := json.Marshal(response)
	s.finish(entry, encoded)

	if s.mode == modeTimeout || s.mode == modeResponseLoss {
		if s.delay > 0 {
			timer := time.NewTimer(s.delay)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				// The result is already durable in this process's idempotency map.
				return
			case <-timer.C:
			}
		}
	}
	if s.mode == modeResponseLoss {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "response loss simulation unavailable", http.StatusNotImplemented)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	writeJSON(w, http.StatusOK, encoded)
}

func (s *gatewaySimulator) begin(requestID, fingerprint string) (*idempotencyEntry, bool, bool) {
	s.mu.Lock()
	if existing, ok := s.requests[requestID]; ok {
		if existing.fingerprint != fingerprint {
			s.mu.Unlock()
			return nil, false, true
		}
		done := existing.done
		s.mu.Unlock()
		<-done
		return existing, true, false
	}
	if len(s.requests) >= s.limit {
		s.mu.Unlock()
		return nil, false, true
	}
	entry := &idempotencyEntry{fingerprint: fingerprint, done: make(chan struct{})}
	s.requests[requestID] = entry
	s.execution++
	s.mu.Unlock()
	return entry, false, false
}

func (s *gatewaySimulator) finish(entry *idempotencyEntry, response []byte) {
	s.mu.Lock()
	entry.response = append([]byte(nil), response...)
	close(entry.done)
	s.mu.Unlock()
}

func (s *gatewaySimulator) executionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execution
}

func validEvaluationRequest(input evaluationRequest) bool {
	return strings.TrimSpace(input.RequestID) != "" && len(input.RequestID) <= 128 &&
		input.Profile == "staging-sim" && strings.TrimSpace(input.RunID) != "" && input.CaseID >= 0 &&
		strings.HasPrefix(input.DatasetURI, "artifact://") && digestPattern.MatchString(input.DatasetSHA256) &&
		strings.TrimSpace(input.DatasetVersion) != "" && strings.TrimSpace(input.ModelProfile) != "" &&
		strings.TrimSpace(input.PromptVersion) != "" && strings.TrimSpace(input.RetrieverVersion) != "" &&
		strings.TrimSpace(input.EvaluatorSchemaVersion) != ""
}

func writeJSON(w http.ResponseWriter, status int, raw []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
