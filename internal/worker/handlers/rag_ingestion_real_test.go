package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
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
		"object_key":       "doc.md",
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
	artifactKey := strings.TrimPrefix(res1.Artifacts["artifact_uri"], "artifact://")
	info, err := h.Store.Head(context.Background(), artifactKey)
	if err != nil {
		t.Fatalf("artifact not written to store: %v", err)
	}
	if info.SHA256 != res1.Artifacts["artifact_sha256"] {
		t.Fatalf("artifact hash mismatch: stored %s, result %s", info.SHA256, res1.Artifacts["artifact_sha256"])
	}
	if info.ContentType != "" && info.ContentType != "application/x-ndjson" {
		t.Fatalf("artifact content type = %q", info.ContentType)
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

func TestRealRAGIngestionUsesSharedStores(t *testing.T) {
	sourceRoot := t.TempDir()
	outputRoot := t.TempDir()
	source, err := artifacts.NewLocalStore(sourceRoot, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	output, err := artifacts.NewLocalStore(outputRoot, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.PutIfAbsent(context.Background(), "docs/doc.md", []byte("First paragraph.\n\nSecond paragraph."), "text/markdown"); err != nil {
		t.Fatal(err)
	}
	h := NewRealRAGIngestionWithStores("", source, output)
	result, err := h.Ingest(context.Background(), map[string]any{
		"document_id": "doc-shared", "object_key": "docs/doc.md", "version": "v1",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Artifacts["artifact_uri"] == "" || result.Artifacts["chunk_count"] != "2" {
		t.Fatalf("unexpected result: %+v", result.Artifacts)
	}
}

func TestRealRAGIngestionRequiresObjectKey(t *testing.T) {
	h := NewRealRAGIngestion(t.TempDir(), t.TempDir())
	if _, err := h.Ingest(context.Background(), map[string]any{"document_id": "x"}); err == nil {
		t.Fatal("expected error when object_key is missing")
	}
}
