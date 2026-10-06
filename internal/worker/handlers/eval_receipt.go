package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

var evalRunIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// gatewayEvalPayload is the versioned, immutable contract used by the external
// evaluator. Secrets deliberately do not appear in this type or in artifacts.
type gatewayEvalPayload struct {
	RunID, DatasetVersion, DatasetURI, DatasetSHA256 string
	ModelProfile, PromptVersion, RetrieverVersion    string
	EvaluatorSchemaVersion                           string
	CaseStart, CaseEnd                               int
}

type evalCaseReceipt struct {
	CaseID         int     `json:"case_id"`
	RequestID      string  `json:"request_id"`
	Status         string  `json:"status"`
	Score          float64 `json:"score,omitempty"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	CostUSD        float64 `json:"cost_usd"`
	ProviderWaitMS float64 `json:"provider_wait_ms"`
	ProviderMS     float64 `json:"provider_duration_ms"`
	Reconciled     bool    `json:"reconciled,omitempty"`
	Error          string  `json:"error,omitempty"`
}

func parseGatewayEvalPayload(payload map[string]any, maxCases int, allowlists map[string]map[string]bool) (gatewayEvalPayload, error) {
	p := gatewayEvalPayload{
		RunID: getString(payload, "run_id", ""), DatasetVersion: getString(payload, "dataset_version", ""),
		DatasetURI: getString(payload, "dataset_uri", ""), DatasetSHA256: getString(payload, "dataset_sha256", ""),
		ModelProfile: getString(payload, "model_profile", ""), PromptVersion: getString(payload, "prompt_version", ""),
		RetrieverVersion: getString(payload, "retriever_version", ""), EvaluatorSchemaVersion: getString(payload, "evaluator_schema_version", ""),
		CaseStart: getInt(payload, "case_start", -1), CaseEnd: getInt(payload, "case_end", -1),
	}
	if getString(payload, "version", "2") != "2" || !evalRunIDPattern.MatchString(p.RunID) ||
		p.DatasetVersion == "" || p.DatasetURI == "" || !regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(p.DatasetSHA256) ||
		p.ModelProfile == "" || p.PromptVersion == "" || p.RetrieverVersion == "" || p.EvaluatorSchemaVersion == "" ||
		p.CaseStart < 0 || p.CaseEnd <= p.CaseStart || p.CaseEnd-p.CaseStart > maxCases {
		return gatewayEvalPayload{}, worker.NewNonRetryableError(fmt.Errorf("gateway eval requires version 2, valid immutable provenance, and a case range of at most %d", maxCases), domain.ErrorTypeBadPayload)
	}
	for name, value := range map[string]string{"dataset_version": p.DatasetVersion, "model_profile": p.ModelProfile, "prompt_version": p.PromptVersion, "retriever_version": p.RetrieverVersion, "evaluator_schema_version": p.EvaluatorSchemaVersion} {
		if allowed := allowlists[name]; len(allowed) > 0 && !allowed[value] {
			return gatewayEvalPayload{}, worker.NewNonRetryableError(fmt.Errorf("%s is not allowlisted", name), domain.ErrorTypeBadPayload)
		}
	}
	return p, nil
}

func (p gatewayEvalPayload) fingerprint() string {
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (p gatewayEvalPayload) requestID(caseID int) string {
	return fmt.Sprintf("%s:%d:%s:%s", p.RunID, caseID, p.ModelProfile, p.PromptVersion)
}

func parseAllowlist(raw string) map[string]bool {
	values := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values[value] = true
		}
	}
	return values
}

func summarizeReceipts(receipts map[int]evalCaseReceipt) (passed, failed, inputTokens, outputTokens int, cost, providerWaitMS, providerMS float64) {
	for _, r := range receipts {
		if r.Status == "passed" {
			passed++
		} else {
			failed++
		}
		inputTokens += r.InputTokens
		outputTokens += r.OutputTokens
		cost += r.CostUSD
		providerWaitMS += r.ProviderWaitMS
		providerMS += r.ProviderMS
	}
	return
}

func orderedReceipts(receipts map[int]evalCaseReceipt) []evalCaseReceipt {
	keys := make([]int, 0, len(receipts))
	for key := range receipts {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	result := make([]evalCaseReceipt, 0, len(keys))
	for _, key := range keys {
		result = append(result, receipts[key])
	}
	return result
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }
