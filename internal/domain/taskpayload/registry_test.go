package taskpayload

import (
	"encoding/json"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestValidateProductionTaskPayloads(t *testing.T) {
	tests := []struct {
		name  string
		type_ domain.TaskType
		data  map[string]any
	}{
		{"rag mock", domain.TaskTypeRAGIngestion, map[string]any{"execution_mode": "mock", "document_id": "doc-1", "mock_duration_ms": 10.0}},
		{"eval deterministic", domain.TaskTypeEval, map[string]any{"execution_mode": "deterministic", "dataset_version": "ds-1", "model_profile": "model-1", "prompt_version": "p-1", "run_id": "run-1", "case_start": 0.0, "case_end": 4.0}},
		{"benchmark", domain.TaskTypeBenchmark, map[string]any{"execution_mode": "mock", "benchmark_name": "claim-latency", "concurrency": 4.0}},
		{"strategy replay", domain.TaskTypeStrategyReplay, map[string]any{"execution_mode": "replay", "proposal_id": "p-1", "job_schema_version": "1", "replay_result_schema_version": "1", "simulator_version": "go-v1", "execution_fingerprint": "abc", "strategy_key": "new_user_feed_exploration", "candidate_config": map[string]any{"exploration_ratio": 0.2}}},
		{"payment effect", domain.TaskTypePayment, map[string]any{"execution_mode": "effect", "operation": "payment", "operation_id": "invoice-1", "target": "customer-1"}},
		{"notification effect", domain.TaskTypeNotification, map[string]any{"execution_mode": "effect", "operation": "notification", "operation_id": "notice-1", "target": "customer-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Validate(tt.type_, domain.TaskPayloadEnvelope{Version: "1", Kind: string(tt.type_), Data: tt.data})
			if err != nil {
				t.Fatal(err)
			}
			got["document_id"] = "mutated"
			if tt.data["document_id"] == "mutated" {
				t.Fatal("validator returned caller-owned data map")
			}
		})
	}
}

func TestValidateRejectsMismatchedAndUnsafePayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload domain.TaskPayloadEnvelope
	}{
		{"wrong kind", domain.TaskPayloadEnvelope{Version: "1", Kind: "eval", Data: map[string]any{"execution_mode": "mock", "document_id": "doc-1"}}},
		{"unknown field", domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"execution_mode": "mock", "document_id": "doc-1", "typo": true}}},
		{"implicit mock fallback", domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-1"}}},
		{"wrong type", domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"execution_mode": "mock", "document_id": 42}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Validate(domain.TaskTypeRAGIngestion, tt.payload); err == nil {
				t.Fatal("Validate() unexpectedly succeeded")
			}
		})
	}
}

func FuzzValidateRAGData(f *testing.F) {
	f.Add([]byte(`{"execution_mode":"mock","document_id":"doc-1"}`))
	f.Add([]byte(`{"execution_mode":true,"document_id":1}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var data map[string]any
		if json.Unmarshal(raw, &data) != nil {
			return
		}
		_, _ = ValidateData(domain.TaskTypeRAGIngestion, data)
	})
}
