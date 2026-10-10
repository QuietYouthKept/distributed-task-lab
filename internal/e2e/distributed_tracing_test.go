package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// TestDistributedTracingContainersExportToCollector drives a real API request,
// PostgreSQL enqueue/claim/ACK, an independent S3-backed Worker, and a fresh
// OTLP Collector. The Compose owner script asserts the Collector's exported
// span text and verifies that payload markers and credentials are absent.
func TestDistributedTracingContainersExportToCollector(t *testing.T) {
	if os.Getenv("E2E_DISTRIBUTED_TRACE_E2E") != "1" {
		t.Skip("E2E_DISTRIBUTED_TRACE_E2E=1 is required for isolated Collector tracing")
	}
	apiURL, apiToken := os.Getenv("E2E_API_URL"), os.Getenv("E2E_API_TOKEN")
	databaseURL, endpoint, bucket := os.Getenv("E2E_DATABASE_URL"), os.Getenv("E2E_MINIO_ENDPOINT"), os.Getenv("E2E_MINIO_BUCKET")
	accessKey, secretKey := os.Getenv("E2E_S3_ACCESS_KEY"), os.Getenv("E2E_S3_SECRET_KEY")
	prefix := os.Getenv("E2E_ARTIFACT_PREFIX")
	for name, value := range map[string]string{
		"E2E_API_URL": apiURL, "E2E_API_TOKEN": apiToken, "E2E_DATABASE_URL": databaseURL,
		"E2E_MINIO_ENDPOINT": endpoint, "E2E_MINIO_BUCKET": bucket,
		"E2E_S3_ACCESS_KEY": accessKey, "E2E_S3_SECRET_KEY": secretKey, "E2E_ARTIFACT_PREFIX": prefix,
	} {
		if value == "" {
			t.Fatalf("required E2E setting %s is missing", name)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open isolated Compose PostgreSQL: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated Compose PostgreSQL: %v", err)
	}
	store, err := artifacts.NewS3Store(artifacts.S3Config{
		Endpoint: endpoint, Bucket: bucket, Prefix: prefix, AccessKey: accessKey, SecretKey: secretKey,
		Region: "us-east-1", MaxBytes: 64 << 20, AllowHTTP: strings.HasPrefix(endpoint, "http://"),
	})
	if err != nil {
		t.Fatalf("configure test S3 client: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("verify isolated MinIO bucket: %v", err)
	}

	runID := uuid.NewString()
	documentID := "trace-doc-" + runID
	inputKey := "trace-input/" + runID + ".md"
	sensitiveMarker := "trace-payload-sensitive-" + strings.ReplaceAll(runID, "-", "")
	input := []byte("Distributed trace artifact fixture.\n\n" + sensitiveMarker)
	if _, err := store.PutIfAbsent(ctx, inputKey, input, "text/markdown"); err != nil {
		t.Fatalf("seed MinIO input: %v", err)
	}
	contentHash := sha256.Sum256(input)
	outputKey := fmt.Sprintf("rag/%s_v1_paragraph_%s.jsonl", documentID, hex.EncodeToString(contentHash[:8]))

	traceID := strings.ReplaceAll(uuid.NewString(), "-", "")
	clientSpanID := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	traceparent := "00-" + traceID + "-" + clientSpanID + "-01"
	requestBody := map[string]any{
		"type": "rag_ingestion",
		"payload": map[string]any{
			"version": "1", "kind": "rag_ingestion",
			"data": map[string]any{
				"execution_mode": "real", "document_id": documentID, "object_key": inputKey,
				"version": "v1", "chunking_profile": "paragraph",
				"_traceparent": "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
			},
		},
		"max_attempts": 3, "timeout_seconds": 30, "retry_delay_seconds": 1,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/v1/tasks", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+apiToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("traceparent", traceparent)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("submit traced task to API: %v", err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	decodeErr := json.Unmarshal(responseBody, &created)
	if response.StatusCode != http.StatusCreated || readErr != nil || decodeErr != nil || closeErr != nil || created.ID == "" {
		t.Fatalf("create response status=%d id_present=%t read_err=%v decode_err=%v close_err=%v", response.StatusCode, created.ID != "", readErr, decodeErr, closeErr)
	}

	var persistedTraceparent string
	if err := db.QueryRowContext(ctx, `SELECT payload->'data'->>'_traceparent' FROM tasks WHERE id=$1`, created.ID).Scan(&persistedTraceparent); err != nil {
		t.Fatalf("read persisted API trace context: %v", err)
	}
	parts := strings.Split(persistedTraceparent, "-")
	if len(parts) != 4 || parts[1] != traceID || parts[2] == clientSpanID || parts[3] != "01" {
		t.Fatalf("persisted trace context was not the trusted API server span: trace_id_match=%t client_span_reused=%t", len(parts) == 4 && parts[1] == traceID, len(parts) == 4 && parts[2] == clientSpanID)
	}

	completed := waitTaskState(t, ctx, apiURL, created.ID, domain.TaskStatusSuccess, 45*time.Second)
	if completed.Attempt != 1 || completed.LeaseVersion != 1 || completed.Result == nil {
		t.Fatalf("unexpected completed task state: status=%s attempt=%d lease_version=%d result_present=%t", completed.Status, completed.Attempt, completed.LeaseVersion, completed.Result != nil)
	}
	info, err := store.Head(ctx, outputKey)
	if err != nil {
		t.Fatalf("verify worker artifact in MinIO: %v", err)
	}
	if completed.Result.Artifacts["artifact_sha256"] != info.SHA256 || info.Size <= 0 {
		t.Fatalf("MinIO result mismatch: result_hash_matches=%t size=%d", completed.Result.Artifacts["artifact_sha256"] == info.SHA256, info.Size)
	}

	t.Logf("TRACE_E2E_TASK_ID=%s TRACE_E2E_TRACE_ID=%s TRACE_E2E_API_SPAN_ID=%s TRACE_E2E_SENSITIVE_MARKER=%s", created.ID, traceID, parts[2], sensitiveMarker)
}
