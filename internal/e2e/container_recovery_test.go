package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// TestAPIWorkerContainersRecoverSharedArtifact exercises the API and two
// independently containerized workers against one real PostgreSQL and MinIO.
// scripts/e2e/run-cross-container.ps1 owns the disposable Compose project.
func TestAPIWorkerContainersRecoverSharedArtifact(t *testing.T) {
	if os.Getenv("REQUIRE_CONTAINER_E2E") != "1" {
		t.Skip("REQUIRE_CONTAINER_E2E=1 is required for cross-container recovery")
	}
	apiURL, apiToken := os.Getenv("E2E_API_URL"), os.Getenv("E2E_API_TOKEN")
	databaseURL, endpoint, bucket := os.Getenv("E2E_DATABASE_URL"), os.Getenv("E2E_MINIO_ENDPOINT"), os.Getenv("E2E_MINIO_BUCKET")
	accessKey, secretKey := os.Getenv("E2E_S3_ACCESS_KEY"), os.Getenv("E2E_S3_SECRET_KEY")
	prefix, project, composeFile := os.Getenv("E2E_ARTIFACT_PREFIX"), os.Getenv("E2E_COMPOSE_PROJECT"), os.Getenv("E2E_COMPOSE_FILE")
	for name, value := range map[string]string{
		"E2E_API_URL": apiURL, "E2E_API_TOKEN": apiToken, "E2E_DATABASE_URL": databaseURL,
		"E2E_MINIO_ENDPOINT": endpoint, "E2E_MINIO_BUCKET": bucket,
		"E2E_S3_ACCESS_KEY": accessKey, "E2E_S3_SECRET_KEY": secretKey,
		"E2E_ARTIFACT_PREFIX": prefix, "E2E_COMPOSE_PROJECT": project, "E2E_COMPOSE_FILE": composeFile,
	} {
		if value == "" {
			t.Fatalf("required E2E setting %s is missing", name)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
		t.Fatalf("configure host-side S3 verifier: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("verify Compose MinIO bucket: %v", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	readyDeadline := time.Now().Add(30 * time.Second)
	var lastReadyError error
	for time.Now().Before(readyDeadline) {
		health, err := client.Get(apiURL + "/ready")
		if err == nil {
			_ = health.Body.Close()
			if health.StatusCode == http.StatusOK {
				lastReadyError = nil
				break
			}
			lastReadyError = fmt.Errorf("readiness status=%d", health.StatusCode)
		} else {
			lastReadyError = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastReadyError != nil {
		t.Fatalf("container API did not become ready within 30s: %v", lastReadyError)
	}

	runID := "container-recovery-" + uuid.NewString()
	documentID := "doc-" + uuid.NewString()
	inputKey := "inputs/" + runID + ".md"
	input := []byte("Cross-container artifact input.\n\nSecond paragraph proves the worker read shared bytes.")
	if _, err := store.PutIfAbsent(ctx, inputKey, input, "text/markdown"); err != nil {
		t.Fatalf("seed input through real MinIO S3 API: %v", err)
	}
	contentHash := sha256.Sum256(input)
	artifactKey := fmt.Sprintf("rag/%s_v1_paragraph_%s.jsonl", documentID, hex.EncodeToString(contentHash[:8]))

	request := map[string]any{
		"type": "rag_ingestion", "payload": map[string]any{
			"version": "1", "kind": "rag_ingestion", "data": map[string]any{
				"execution_mode": "real", "document_id": documentID, "object_key": inputKey,
				"version": "v1", "chunking_profile": "paragraph",
			},
		}, "max_attempts": 3, "timeout_seconds": 30, "retry_delay_seconds": 1,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/v1/tasks", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Authorization", "Bearer "+apiToken)
	createReq.Header.Set("Content-Type", "application/json")
	response, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("create task through container API: %v", err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	decodeErr := json.Unmarshal(responseBody, &created)
	if response.StatusCode != http.StatusCreated || readErr != nil || decodeErr != nil || closeErr != nil || created.ID == "" {
		t.Fatalf("create response status=%d id_present=%t response=%q read_err=%v decode_err=%v close_err=%v", response.StatusCode, created.ID != "", responseBody, readErr, decodeErr, closeErr)
	}

	tx, owner := lockClaimedTask(t, ctx, db, created.ID, 30*time.Second)
	defer func() { _ = tx.Rollback() }()
	claimed := getTask(t, ctx, apiURL, created.ID)
	if claimed.Status != domain.TaskStatusRunning || claimed.LeasedBy == nil || *claimed.LeasedBy != owner || claimed.Attempt != 1 || claimed.LeaseVersion != 1 {
		t.Fatalf("unexpected first container lease: status=%s owner=%v attempt=%d generation=%d", claimed.Status, claimed.LeasedBy, claimed.Attempt, claimed.LeaseVersion)
	}
	data, err := waitArtifact(ctx, store, artifactKey, 30*time.Second)
	if err != nil {
		t.Fatalf("wait for Worker A S3 Put before ACK: %v", err)
	}
	info, err := store.Head(ctx, artifactKey)
	if err != nil {
		t.Fatalf("read Worker A artifact metadata from MinIO: %v", err)
	}
	if info.SHA256 == "" || info.Size != int64(len(data)) {
		t.Fatalf("MinIO object metadata is incomplete: sha_present=%t size=%d bytes=%d", info.SHA256 != "", info.Size, len(data))
	}
	if stillRunning := getTask(t, ctx, apiURL, created.ID); stillRunning.Status != domain.TaskStatusRunning {
		t.Fatalf("task after MinIO Put but before ACK barrier release is %s", stillRunning.Status)
	}

	service := "worker-a"
	if owner == "e2e-worker-b" {
		service = "worker-b"
	}
	if owner != "e2e-worker-a" && owner != "e2e-worker-b" {
		t.Fatalf("unexpected Compose worker owner %q", owner)
	}
	kill := exec.CommandContext(ctx, "docker", "compose", "--project-name", project, "--file", composeFile, "kill", service)
	if output, err := kill.CombinedOutput(); err != nil {
		t.Fatalf("kill only ACK-blocked Compose service %s (diagnostic bytes=%d): %v", service, len(output), err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("release row lock after killing owner %s: %v", owner, err)
	}

	completed := waitTaskState(t, ctx, apiURL, created.ID, domain.TaskStatusSuccess, 45*time.Second)
	if completed.Attempt != 2 || completed.LeaseVersion != 2 || completed.LeasedBy != nil {
		t.Fatalf("terminal task state leased_by=%v attempt=%d generation=%d; want lease cleared, attempt=2, generation=2", completed.LeasedBy, completed.Attempt, completed.LeaseVersion)
	}
	var reclaimedBy string
	if err := db.QueryRowContext(ctx, `SELECT worker_id FROM task_events WHERE task_id=$1 AND event_type='claimed' ORDER BY id DESC LIMIT 1`, created.ID).Scan(&reclaimedBy); err != nil {
		t.Fatalf("read recovery claim event: %v", err)
	}
	if reclaimedBy == owner {
		t.Fatalf("task was reclaimed by killed owner %q", owner)
	}
	if completed.Result == nil || completed.Result.Artifacts["idempotent_reused"] != "true" {
		t.Fatalf("reclaim did not report reuse of existing shared artifact: %+v", completed.Result)
	}
	stored, storedInfo, err := store.Get(ctx, artifactKey)
	if err != nil {
		t.Fatalf("independently read output object from MinIO: %v", err)
	}
	if !bytes.Equal(stored, data) || storedInfo.SHA256 != info.SHA256 || completed.Result.Artifacts["artifact_sha256"] != info.SHA256 {
		t.Fatalf("artifact changed or database reference mismatches object: initial=%s current=%s result=%s", info.SHA256, storedInfo.SHA256, completed.Result.Artifacts["artifact_sha256"])
	}
	staleErr := postgres.NewPostgresQueue(db).MarkSuccess(ctx, created.ID, owner, claimed.LeaseVersion, domain.TaskResult{Summary: "stale container generation"})
	if !errors.Is(staleErr, domain.ErrTaskNotClaimed) {
		t.Fatalf("stale generation ACK returned %v, want ErrTaskNotClaimed", staleErr)
	}
	var successEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM task_events WHERE task_id=$1 AND event_type='succeeded'`, created.ID).Scan(&successEvents); err != nil {
		t.Fatalf("count terminal success events: %v", err)
	}
	if successEvents != 1 {
		t.Fatalf("task has %d success events, want exactly one", successEvents)
	}
	t.Logf("PASS: isolated API, PostgreSQL, MinIO and two Worker containers; owner=%s killed after Put/before ACK; reclaimed_by=%s attempt=%d generation=%d; artifact_sha256=%s; stale generation fenced; one success event", owner, reclaimedBy, completed.Attempt, completed.LeaseVersion, info.SHA256)
}
