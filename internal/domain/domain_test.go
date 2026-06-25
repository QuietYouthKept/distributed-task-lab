package domain

import (
	"encoding/json"
	"testing"
)

func TestTaskStatusValidation(t *testing.T) {
	tests := []struct {
		status TaskStatus
		valid  bool
	}{
		{TaskStatusPending, true},
		{TaskStatusRunning, true},
		{TaskStatusSuccess, true},
		{TaskStatusFailed, true},
		{TaskStatusDLQ, true},
		{TaskStatus("unknown"), false},
		{TaskStatus(""), false},
	}

	for _, tt := range tests {
		got := ValidTaskStatuses[tt.status]
		if got != tt.valid {
			t.Errorf("ValidTaskStatuses[%q] = %v, want %v", tt.status, got, tt.valid)
		}
	}
}

func TestTaskTypeValidation(t *testing.T) {
	tests := []struct {
		typ   TaskType
		valid bool
	}{
		{TaskTypeRAGIngestion, true},
		{TaskTypeEval, true},
		{TaskTypeBenchmark, true},
		{TaskType("unknown"), false},
		{TaskType(""), false},
	}

	for _, tt := range tests {
		got := ValidTaskTypes[tt.typ]
		if got != tt.valid {
			t.Errorf("ValidTaskTypes[%q] = %v, want %v", tt.typ, got, tt.valid)
		}
	}
}

func TestTaskPayloadEnvelopeParsing(t *testing.T) {
	raw := `{
		"version": "1",
		"kind": "rag_ingestion",
		"data": {
			"document_id": "doc-001",
			"dataset_id": "ds-demo",
			"source_uri": "s3://mock/docs.pdf",
			"chunking_profile": "default",
			"embedding_profile": "text-embedding-mock",
			"index_name": "demo-index"
		}
	}`

	var env TaskPayloadEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if env.Version != "1" {
		t.Errorf("Version = %q, want %q", env.Version, "1")
	}
	if env.Kind != "rag_ingestion" {
		t.Errorf("Kind = %q, want %q", env.Kind, "rag_ingestion")
	}

	data := env.Data
	if data["document_id"] != "doc-001" {
		t.Errorf("document_id = %q, want %q", data["document_id"], "doc-001")
	}
	if data["dataset_id"] != "ds-demo" {
		t.Errorf("dataset_id = %q, want %q", data["dataset_id"], "ds-demo")
	}
	if data["source_uri"] != "s3://mock/docs.pdf" {
		t.Errorf("source_uri = %q, want %q", data["source_uri"], "s3://mock/docs.pdf")
	}
	if data["chunking_profile"] != "default" {
		t.Errorf("chunking_profile = %q, want %q", data["chunking_profile"], "default")
	}
	if data["embedding_profile"] != "text-embedding-mock" {
		t.Errorf("embedding_profile = %q, want %q", data["embedding_profile"], "text-embedding-mock")
	}
	if data["index_name"] != "demo-index" {
		t.Errorf("index_name = %q, want %q", data["index_name"], "demo-index")
	}
}

func TestTaskResultMarshaling(t *testing.T) {
	result := TaskResult{
		Summary: "RAG ingestion completed",
		Artifacts: map[string]string{
			"document_id": "doc-001",
			"chunk_count": "42",
		},
		Metrics: map[string]float64{
			"processed_documents": 1,
			"generated_chunks":    42,
		},
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var parsed TaskResult
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if parsed.Summary != result.Summary {
		t.Errorf("Summary = %q, want %q", parsed.Summary, result.Summary)
	}
	if parsed.Artifacts["document_id"] != "doc-001" {
		t.Errorf("Artifacts[document_id] = %q, want %q", parsed.Artifacts["document_id"], "doc-001")
	}
	if parsed.Metrics["generated_chunks"] != 42 {
		t.Errorf("Metrics[generated_chunks] = %v, want %v", parsed.Metrics["generated_chunks"], 42)
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.RetryDelaySeconds != 5 {
		t.Errorf("RetryDelaySeconds = %d, want 5", p.RetryDelaySeconds)
	}
}
