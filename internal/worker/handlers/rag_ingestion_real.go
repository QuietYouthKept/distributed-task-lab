package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

// Chunk is a single document chunk with a deterministic embedding.
type Chunk struct {
	DocumentID string   `json:"document_id"`
	Index      int      `json:"index"`
	Text       string   `json:"text"`
	Embedding  []string `json:"embedding"`
}

// RealRAGIngestion implements a lightweight but real RAG ingestion pipeline:
// read a markdown file, chunk by paragraph, generate deterministic hash
// embeddings, and write a JSONL artifact. It is idempotent on
// document_id + version + chunking_profile + content hash.
type RealRAGIngestion struct {
	DocDir      string
	ArtifactDir string
}

// NewRealRAGIngestion creates a real RAG ingestion pipeline.
func NewRealRAGIngestion(docDir, artifactDir string) *RealRAGIngestion {
	return &RealRAGIngestion{DocDir: docDir, ArtifactDir: artifactDir}
}

// Ingest runs the pipeline for a payload describing a document to ingest.
func (h *RealRAGIngestion) Ingest(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	docID := getString(payload, "document_id", "")
	sourcePath := getString(payload, "source_path", "")
	version := getString(payload, "version", "v1")
	profile := getString(payload, "chunking_profile", "paragraph")

	if docID == "" || sourcePath == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("document_id and source_path are required"), domain.ErrorTypeBadPayload)
	}

	fullPath := sourcePath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(h.DocDir, sourcePath)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, worker.NewNonRetryableError(fmt.Errorf("read document: %w", err), domain.ErrorTypeBadPayload)
	}

	sum := sha256.Sum256(data)
	key := fmt.Sprintf("%s_%s_%s_%s", docID, version, profile, hex.EncodeToString(sum[:8]))
	artifactName := key + ".jsonl"
	artifactPath := filepath.Join(h.ArtifactDir, artifactName)

	chunks := chunkParagraphs(docID, string(data))
	reused := false
	if _, err := os.Stat(artifactPath); err == nil {
		reused = true
	} else if err := h.writeArtifact(artifactPath, chunks); err != nil {
		return nil, fmt.Errorf("write artifact: %w", err)
	}

	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("RAG ingestion for document %s: %d chunks (reused=%t)", docID, len(chunks), reused),
		Artifacts: map[string]string{
			"document_id":       docID,
			"artifact_uri":      artifactPath,
			"chunk_count":       fmt.Sprintf("%d", len(chunks)),
			"embedding_model":   "deterministic-sha256",
			"chunking_profile":  profile,
			"idempotent_reused": fmt.Sprintf("%t", reused),
		},
		Metrics: map[string]float64{
			"generated_chunks": float64(len(chunks)),
		},
	}, nil
}

// writeArtifact writes chunks as newline-delimited JSON.
func (h *RealRAGIngestion) writeArtifact(path string, chunks []Chunk) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, c := range chunks {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return nil
}

// chunkParagraphs splits text into paragraphs and assigns deterministic embeddings.
func chunkParagraphs(docID, text string) []Chunk {
	raw := strings.Split(text, "\n\n")
	var chunks []Chunk
	idx := 0
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		chunks = append(chunks, Chunk{
			DocumentID: docID,
			Index:      idx,
			Text:       p,
			Embedding:  embed(p),
		})
		idx++
	}
	return chunks
}

// embed returns a deterministic 8-component embedding for a chunk.
func embed(text string) []string {
	sum := sha256.Sum256([]byte(text))
	parts := make([]string, 8)
	for i := 0; i < 8; i++ {
		parts[i] = hex.EncodeToString(sum[i*4 : i*4+4])
	}
	return parts
}
