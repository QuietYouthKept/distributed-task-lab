package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRealRAGIngestion(t *testing.T) {
	docDir := t.TempDir()
	artifactDir := t.TempDir()

	doc := "Paragraph one.\n\nParagraph two.\n\nParagraph three."
	if err := os.WriteFile(filepath.Join(docDir, "doc.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewRealRAGIngestion(docDir, artifactDir)
	payload := map[string]any{
		"document_id":      "doc-001",
		"source_path":      "doc.md",
		"version":          "v1",
		"chunking_profile": "paragraph",
	}

	// First ingestion: creates the artifact.
	res1, err := h.Ingest(context.Background(), payload)
	if err != nil {
		t.Fatalf("Ingest 1: %v", err)
	}
	if res1.Artifacts["chunk_count"] != "3" {
		t.Errorf("chunk_count = %s, want 3", res1.Artifacts["chunk_count"])
	}
	if res1.Artifacts["idempotent_reused"] != "false" {
		t.Error("first ingest should not be reused")
	}
	if _, err := os.Stat(res1.Artifacts["artifact_uri"]); err != nil {
		t.Errorf("artifact not written: %v", err)
	}

	// Second ingestion: reuses the existing artifact.
	res2, err := h.Ingest(context.Background(), payload)
	if err != nil {
		t.Fatalf("Ingest 2: %v", err)
	}
	if res2.Artifacts["idempotent_reused"] != "true" {
		t.Error("second ingest should be reused")
	}
	if res2.Artifacts["artifact_uri"] != res1.Artifacts["artifact_uri"] {
		t.Error("artifact URI should be stable across identical ingests")
	}
}

func TestRealRAGIngestionRequiresSource(t *testing.T) {
	h := NewRealRAGIngestion(t.TempDir(), t.TempDir())
	if _, err := h.Ingest(context.Background(), map[string]any{"document_id": "x"}); err == nil {
		t.Fatal("expected error when source_path is missing")
	}
}
