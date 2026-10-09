package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
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
	SourceStore artifacts.Store
	Store       artifacts.Store
	storeErr    error
}

// NewRealRAGIngestion creates a real RAG ingestion pipeline.
func NewRealRAGIngestion(docDir, artifactDir string) *RealRAGIngestion {
	store, err := artifacts.NewLocalStore(artifactDir, "", 64<<20)
	return &RealRAGIngestion{DocDir: docDir, ArtifactDir: artifactDir, Store: store, storeErr: err}
}

func NewRealRAGIngestionWithStores(docDir string, sourceStore, artifactStore artifacts.Store) *RealRAGIngestion {
	return &RealRAGIngestion{DocDir: docDir, SourceStore: sourceStore, Store: artifactStore}
}

// Ingest runs the pipeline for a payload describing a document to ingest.
func (h *RealRAGIngestion) Ingest(ctx context.Context, payload map[string]any) (*worker.TaskExecResult, error) {
	docID := getString(payload, "document_id", "")
	objectKey := getString(payload, "object_key", "")
	version := getString(payload, "version", "v1")
	profile := getString(payload, "chunking_profile", "paragraph")

	if docID == "" || objectKey == "" {
		return nil, worker.NewNonRetryableError(fmt.Errorf("document_id and object_key are required"), domain.ErrorTypeBadPayload)
	}
	if h.storeErr != nil {
		return nil, fmt.Errorf("initialize artifact store: %w", h.storeErr)
	}
	if h.Store == nil {
		return nil, fmt.Errorf("artifact store is not configured")
	}

	var data []byte
	var err error
	if h.SourceStore != nil {
		data, _, err = h.SourceStore.Get(ctx, objectKey)
	} else {
		var fullPath string
		fullPath, err = artifacts.ResolveLocalObjectKey(h.DocDir, objectKey)
		if err == nil {
			data, err = os.ReadFile(fullPath)
		}
	}
	if err != nil {
		return nil, worker.NewNonRetryableError(fmt.Errorf("read document through controlled source reference: %w", err), domain.ErrorTypeBadPayload)
	}

	sum := sha256.Sum256(data)
	key := fmt.Sprintf("%s_%s_%s_%s", docID, version, profile, hex.EncodeToString(sum[:8]))
	artifactName := key + ".jsonl"
	artifactKey := "rag/" + artifactName

	chunks := chunkParagraphs(docID, string(data))
	var encoded bytes.Buffer
	enc := json.NewEncoder(&encoded)
	for _, chunk := range chunks {
		if err := enc.Encode(chunk); err != nil {
			return nil, fmt.Errorf("encode chunks: %w", err)
		}
	}
	_, headErr := h.Store.Head(ctx, artifactKey)
	reused := headErr == nil
	if headErr != nil && !errors.Is(headErr, artifacts.ErrNotFound) {
		return nil, fmt.Errorf("check existing artifact: %w", headErr)
	}
	info, err := h.Store.PutIfAbsent(ctx, artifactKey, encoded.Bytes(), "application/x-ndjson")
	if err != nil {
		return nil, fmt.Errorf("write artifact: %w", err)
	}
	if info.SHA256 == "" || info.Size != int64(encoded.Len()) {
		return nil, fmt.Errorf("artifact store returned invalid integrity metadata")
	}

	return &worker.TaskExecResult{
		Summary: fmt.Sprintf("RAG ingestion for document %s: %d chunks", docID, len(chunks)),
		Artifacts: map[string]string{
			"document_id":       docID,
			"artifact_uri":      "artifact://" + info.Key,
			"artifact_sha256":   info.SHA256,
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
