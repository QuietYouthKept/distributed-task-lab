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
	t.Logf("PASS: task_id=%s; isolated API, PostgreSQL, MinIO and two Worker containers; owner=%s killed after Put/before ACK; reclaimed_by=%s attempt=%d generation=%d; artifact_sha256=%s; stale generation fenced; one success event", created.ID, owner, reclaimedBy, completed.Attempt, completed.LeaseVersion, info.SHA256)
}

// TestAPIWorkerContainerRecoversLostPutResponse sends the Worker's real S3
// traffic through the isolated Compose fault proxy. MinIO commits the output,
// but the proxy drops the successful PUT and verification HEAD responses. The
// first worker records a retry; the other container then reclaims the task and
// verifies/reuses the durable object through the same shared MinIO service.
func TestAPIWorkerContainerRecoversLostPutResponse(t *testing.T) {
	if os.Getenv("REQUIRE_CONTAINER_E2E") != "1" {
		t.Skip("REQUIRE_CONTAINER_E2E=1 is required for cross-container recovery")
	}
	apiURL, apiToken := os.Getenv("E2E_API_URL"), os.Getenv("E2E_API_TOKEN")
	databaseURL, endpoint, bucket := os.Getenv("E2E_DATABASE_URL"), os.Getenv("E2E_MINIO_ENDPOINT"), os.Getenv("E2E_MINIO_BUCKET")
	accessKey, secretKey := os.Getenv("E2E_S3_ACCESS_KEY"), os.Getenv("E2E_S3_SECRET_KEY")
	prefix, project, composeFile := os.Getenv("E2E_ARTIFACT_PREFIX"), os.Getenv("E2E_COMPOSE_PROJECT"), os.Getenv("E2E_COMPOSE_FILE")
	proxyAdminURL, proxyToken := os.Getenv("E2E_S3_PROXY_ADMIN_URL"), os.Getenv("E2E_S3_PROXY_TOKEN")
	for name, value := range map[string]string{
		"E2E_API_URL": apiURL, "E2E_API_TOKEN": apiToken, "E2E_DATABASE_URL": databaseURL,
		"E2E_MINIO_ENDPOINT": endpoint, "E2E_MINIO_BUCKET": bucket,
		"E2E_S3_ACCESS_KEY": accessKey, "E2E_S3_SECRET_KEY": secretKey,
		"E2E_ARTIFACT_PREFIX": prefix, "E2E_COMPOSE_PROJECT": project, "E2E_COMPOSE_FILE": composeFile,
		"E2E_S3_PROXY_ADMIN_URL": proxyAdminURL, "E2E_S3_PROXY_TOKEN": proxyToken,
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

	compose := func(args ...string) {
		t.Helper()
		all := []string{"compose", "--project-name", project, "--file", composeFile}
		all = append(all, args...)
		output, err := exec.CommandContext(ctx, "docker", all...).CombinedOutput()
		if err != nil {
			t.Fatalf("scoped Compose %s failed (output bytes=%d): %v", strings.Join(args, " "), len(output), err)
		}
	}
	compose("up", "--detach", "--wait", "--wait-timeout", "120", "worker-a", "worker-b")
	compose("stop", "worker-b")

	runID := "lost-put-response-" + uuid.NewString()
	documentID := "doc-" + uuid.NewString()
	inputKey := "inputs/" + runID + ".md"
	input := []byte("Cross-container response-loss input.\n\nThe next paragraph is shared through MinIO.")
	if _, err := store.PutIfAbsent(ctx, inputKey, input, "text/markdown"); err != nil {
		t.Fatalf("seed input through real MinIO S3 API: %v", err)
	}
	contentHash := sha256.Sum256(input)
	artifactKey := fmt.Sprintf("rag/%s_v1_paragraph_%s.jsonl", documentID, hex.EncodeToString(contentHash[:8]))
	armBody, err := json.Marshal(map[string]any{"object_path": bucket + "/" + prefix + "/" + artifactKey, "drop_responses": true})
	if err != nil {
		t.Fatal(err)
	}
	armRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyAdminURL+"/arm", bytes.NewReader(armBody))
	if err != nil {
		t.Fatalf("build S3 fault arm request: %v", err)
	}
	armRequest.Header.Set("Authorization", "Bearer "+proxyToken)
	armRequest.Header.Set("Content-Type", "application/json")
	armResponse, err := http.DefaultClient.Do(armRequest)
	if err != nil {
		t.Fatalf("arm isolated S3 response-loss fault: %v", err)
	}
	_ = armResponse.Body.Close()
	if armResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("arm S3 response-loss fault returned HTTP %d", armResponse.StatusCode)
	}

	request := map[string]any{
		"type": "rag_ingestion", "payload": map[string]any{
			"version": "1", "kind": "rag_ingestion", "data": map[string]any{
				"execution_mode": "real", "document_id": documentID, "object_key": inputKey,
				"version": "v1", "chunking_profile": "paragraph",
			},
		}, "max_attempts": 3, "timeout_seconds": 30, "retry_delay_seconds": 15,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/v1/tasks", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build task create request: %v", err)
	}
	createReq.Header.Set("Authorization", "Bearer "+apiToken)
	createReq.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create response-loss task through container API: %v", err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	var created struct {
		ID string `json:"id"`
	}
	decodeErr := json.Unmarshal(responseBody, &created)
	if response.StatusCode != http.StatusCreated || readErr != nil || closeErr != nil || decodeErr != nil || created.ID == "" {
		t.Fatalf("create response status=%d id_present=%t read_err=%v decode_err=%v close_err=%v", response.StatusCode, created.ID != "", readErr, decodeErr, closeErr)
	}

	deadline := time.Now().Add(30 * time.Second)
	var fault struct {
		PutDropped  bool `json:"put_dropped"`
		HeadDropped bool `json:"head_dropped"`
	}
	for time.Now().Before(deadline) {
		faultRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyAdminURL+"/state", nil)
		if err != nil {
			t.Fatalf("build S3 fault state request: %v", err)
		}
		faultRequest.Header.Set("Authorization", "Bearer "+proxyToken)
		faultResponse, err := http.DefaultClient.Do(faultRequest)
		if err == nil {
			decodeErr := json.NewDecoder(faultResponse.Body).Decode(&fault)
			_ = faultResponse.Body.Close()
			if faultResponse.StatusCode != http.StatusOK || decodeErr != nil {
				t.Fatalf("read S3 fault state: status=%d decode_err=%v", faultResponse.StatusCode, decodeErr)
			}
			if fault.PutDropped && fault.HeadDropped {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !fault.PutDropped || !fault.HeadDropped {
		t.Fatalf("S3 fault proxy did not drop both Worker responses: put=%t head=%t", fault.PutDropped, fault.HeadDropped)
	}

	data, info, err := waitArtifactInfo(ctx, store, artifactKey, 20*time.Second)
	if err != nil {
		t.Fatalf("independently verify object persisted by MinIO despite lost Worker responses: %v", err)
	}
	waitRetryEvent(t, ctx, db, created.ID, 30*time.Second)
	firstAttempt := getTask(t, ctx, apiURL, created.ID)
	if firstAttempt.Status != domain.TaskStatusPending || firstAttempt.Attempt != 1 || firstAttempt.LeaseVersion != 1 {
		t.Fatalf("first response-loss retry state=%s attempt=%d generation=%d; want pending/1/1", firstAttempt.Status, firstAttempt.Attempt, firstAttempt.LeaseVersion)
	}
	disarmRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyAdminURL+"/disarm", nil)
	if err != nil {
		t.Fatalf("build S3 fault disarm request: %v", err)
	}
	disarmRequest.Header.Set("Authorization", "Bearer "+proxyToken)
	disarmResponse, err := http.DefaultClient.Do(disarmRequest)
	if err != nil {
		t.Fatalf("disarm isolated S3 response-loss fault after retry was persisted: %v", err)
	}
	_ = disarmResponse.Body.Close()
	if disarmResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("disarm S3 response-loss fault returned HTTP %d", disarmResponse.StatusCode)
	}
	compose("stop", "worker-a")
	compose("start", "worker-b")
	completed := waitTaskState(t, ctx, apiURL, created.ID, domain.TaskStatusSuccess, 45*time.Second)
	if completed.Attempt != 2 || completed.LeaseVersion != 2 || completed.LeasedBy != nil {
		t.Fatalf("recovered task state leased_by=%v attempt=%d generation=%d; want lease cleared, attempt=2, generation=2", completed.LeasedBy, completed.Attempt, completed.LeaseVersion)
	}
	var reclaimedBy string
	if err := db.QueryRowContext(ctx, `SELECT worker_id FROM task_events WHERE task_id=$1 AND event_type='claimed' ORDER BY id DESC LIMIT 1`, created.ID).Scan(&reclaimedBy); err != nil {
		t.Fatalf("read recovery claim event: %v", err)
	}
	if reclaimedBy != "e2e-worker-b" {
		t.Fatalf("task reclaimed by %q, want isolated e2e-worker-b", reclaimedBy)
	}
	if completed.Result == nil || completed.Result.Artifacts["idempotent_reused"] != "true" || completed.Result.Artifacts["artifact_sha256"] != info.SHA256 {
		t.Fatalf("recovered Worker did not report reuse of the MinIO object: result=%+v expected_sha256=%s", completed.Result, info.SHA256)
	}
	stored, storedInfo, err := store.Get(ctx, artifactKey)
	if err != nil || !bytes.Equal(stored, data) || storedInfo.SHA256 != info.SHA256 {
		t.Fatalf("MinIO object changed across retry: bytes_equal=%t original_sha=%s current_sha=%s err=%v", bytes.Equal(stored, data), info.SHA256, storedInfo.SHA256, err)
	}
	var retriedEvents, successEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE event_type='retried'), count(*) FILTER (WHERE event_type='succeeded') FROM task_events WHERE task_id=$1`, created.ID).Scan(&retriedEvents, &successEvents); err != nil {
		t.Fatalf("count response-loss and success events: %v", err)
	}
	if retriedEvents != 1 || successEvents != 1 {
		t.Fatalf("task event counts retried=%d succeeded=%d; want exactly 1/1", retriedEvents, successEvents)
	}
	t.Logf("PASS: Worker A PUT persisted but PUT/HEAD responses were dropped; PostgreSQL recorded one retry; Worker B reclaimed attempt=%d generation=%d and reused MinIO SHA256=%s", completed.Attempt, completed.LeaseVersion, info.SHA256)
}

// TestTwoWorkerContainersRaceForSameArtifact prevents the first output PUT
// from reaching MinIO until a second container has reached the same-key PUT.
// This proves conditional object creation is the cross-process arbiter rather
// than either Worker's local memory.
func TestTwoWorkerContainersRaceForSameArtifact(t *testing.T) {
	if os.Getenv("REQUIRE_CONTAINER_E2E") != "1" {
		t.Skip("REQUIRE_CONTAINER_E2E=1 is required for cross-container recovery")
	}
	apiURL, apiToken := os.Getenv("E2E_API_URL"), os.Getenv("E2E_API_TOKEN")
	databaseURL, endpoint, bucket := os.Getenv("E2E_DATABASE_URL"), os.Getenv("E2E_MINIO_ENDPOINT"), os.Getenv("E2E_MINIO_BUCKET")
	accessKey, secretKey := os.Getenv("E2E_S3_ACCESS_KEY"), os.Getenv("E2E_S3_SECRET_KEY")
	prefix, project, composeFile := os.Getenv("E2E_ARTIFACT_PREFIX"), os.Getenv("E2E_COMPOSE_PROJECT"), os.Getenv("E2E_COMPOSE_FILE")
	proxyAdminURL, proxyToken := os.Getenv("E2E_S3_PROXY_ADMIN_URL"), os.Getenv("E2E_S3_PROXY_TOKEN")
	for name, value := range map[string]string{
		"E2E_API_URL": apiURL, "E2E_API_TOKEN": apiToken, "E2E_DATABASE_URL": databaseURL,
		"E2E_MINIO_ENDPOINT": endpoint, "E2E_MINIO_BUCKET": bucket,
		"E2E_S3_ACCESS_KEY": accessKey, "E2E_S3_SECRET_KEY": secretKey,
		"E2E_ARTIFACT_PREFIX": prefix, "E2E_COMPOSE_PROJECT": project, "E2E_COMPOSE_FILE": composeFile,
		"E2E_S3_PROXY_ADMIN_URL": proxyAdminURL, "E2E_S3_PROXY_TOKEN": proxyToken,
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
	composeArgs := []string{"compose", "--project-name", project, "--file", composeFile, "up", "--detach", "--wait", "--wait-timeout", "120", "worker-a", "worker-b"}
	composeOutput, err := exec.CommandContext(ctx, "docker", composeArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("start both isolated Worker containers (diagnostic bytes=%d): %v", len(composeOutput), err)
	}
	stopWorkerB := []string{"compose", "--project-name", project, "--file", composeFile, "stop", "worker-b"}
	if output, err := exec.CommandContext(ctx, "docker", stopWorkerB...).CombinedOutput(); err != nil {
		t.Fatalf("stop Worker B before deterministic first claim (diagnostic bytes=%d): %v", len(output), err)
	}

	runID := "same-artifact-race-" + uuid.NewString()
	documentID := "doc-" + uuid.NewString()
	inputKey := "inputs/" + runID + ".md"
	input := []byte("Both workers process the same immutable document.\n\nThis paragraph creates identical JSONL output.")
	if _, err := store.PutIfAbsent(ctx, inputKey, input, "text/markdown"); err != nil {
		t.Fatalf("seed input through real MinIO S3 API: %v", err)
	}
	contentHash := sha256.Sum256(input)
	artifactKey := fmt.Sprintf("rag/%s_v1_paragraph_%s.jsonl", documentID, hex.EncodeToString(contentHash[:8]))
	armBody, err := json.Marshal(map[string]any{
		"object_path":       bucket + "/" + prefix + "/" + artifactKey,
		"put_barrier_count": 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	armRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyAdminURL+"/arm", bytes.NewReader(armBody))
	if err != nil {
		t.Fatalf("build S3 two-writer barrier request: %v", err)
	}
	armRequest.Header.Set("Authorization", "Bearer "+proxyToken)
	armRequest.Header.Set("Content-Type", "application/json")
	armResponse, err := http.DefaultClient.Do(armRequest)
	if err != nil {
		t.Fatalf("arm isolated S3 two-writer barrier: %v", err)
	}
	_ = armResponse.Body.Close()
	if armResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("arm S3 two-writer barrier returned HTTP %d", armResponse.StatusCode)
	}

	createTask := func(taskDocumentID string) string {
		t.Helper()
		payload := map[string]any{
			"type": "rag_ingestion", "payload": map[string]any{
				"version": "1", "kind": "rag_ingestion", "data": map[string]any{
					"execution_mode": "real", "document_id": taskDocumentID, "object_key": inputKey,
					"version": "v1", "chunking_profile": "paragraph",
				},
			}, "max_attempts": 3, "timeout_seconds": 30,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal same-case task: %v", err)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/v1/tasks", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("build same-case task request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+apiToken)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("create same-case task through container API: %v", err)
		}
		defer response.Body.Close()
		var created struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil || response.StatusCode != http.StatusCreated || created.ID == "" {
			t.Fatalf("create same-case task status=%d id_present=%t decode_err=%v", response.StatusCode, created.ID != "", err)
		}
		return created.ID
	}
	readBarrierArrivals := func(target int) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyAdminURL+"/state", nil)
			if err != nil {
				t.Fatalf("build two-writer barrier state request: %v", err)
			}
			request.Header.Set("Authorization", "Bearer "+proxyToken)
			response, err := http.DefaultClient.Do(request)
			if err == nil {
				var barrier struct {
					PutBarrierArrivals int `json:"put_barrier_arrivals"`
				}
				decodeErr := json.NewDecoder(response.Body).Decode(&barrier)
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK || decodeErr != nil {
					t.Fatalf("read two-writer barrier state status=%d decode_err=%v", response.StatusCode, decodeErr)
				}
				if barrier.PutBarrierArrivals >= target {
					return
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("fewer than %d Worker PUT requests reached the same-object barrier", target)
	}
	taskOne := createTask(documentID)
	readBarrierArrivals(1)
	queuedTaskDocumentID := "doc-" + uuid.NewString()
	queuedTask := createTask(queuedTaskDocumentID)
	claimDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(claimDeadline) {
		var queuedOwner string
		err := db.QueryRowContext(ctx, `SELECT worker_id FROM task_events WHERE task_id=$1 AND event_type='claimed' ORDER BY id LIMIT 1`, queuedTask).Scan(&queuedOwner)
		if err == nil {
			if queuedOwner != "e2e-worker-a" {
				t.Fatalf("Worker A prefetch-capacity filler was claimed by %q", queuedOwner)
			}
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("read Worker A prefetch-capacity filler claim: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	var fillerOwner string
	if err := db.QueryRowContext(ctx, `SELECT worker_id FROM task_events WHERE task_id=$1 AND event_type='claimed' ORDER BY id LIMIT 1`, queuedTask).Scan(&fillerOwner); err != nil || fillerOwner != "e2e-worker-a" {
		t.Fatalf("Worker A did not reserve its second local claim slot: owner=%q err=%v", fillerOwner, err)
	}
	startWorkerB := []string{"compose", "--project-name", project, "--file", composeFile, "start", "worker-b"}
	if output, err := exec.CommandContext(ctx, "docker", startWorkerB...).CombinedOutput(); err != nil {
		t.Fatalf("start Worker B after Worker A reached same-object barrier (diagnostic bytes=%d): %v", len(output), err)
	}
	taskTwo := createTask(documentID)
	if taskOne == taskTwo {
		t.Fatal("API returned the same task identity for duplicate same-case requests")
	}
	readBarrierArrivals(2)

	waitSuccess := func(taskID string) *domain.Task {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		var latest *domain.Task
		for time.Now().Before(deadline) {
			latest = getTask(t, ctx, apiURL, taskID)
			if latest.Status == domain.TaskStatusSuccess {
				return latest
			}
			if latest.Status == domain.TaskStatusFailed || latest.Status == domain.TaskStatusDLQ {
				lastError := "<none>"
				if latest.LastError != nil {
					lastError = *latest.LastError
				}
				t.Fatalf("same-case task %s reached %s after attempt=%d generation=%d: %s", taskID, latest.Status, latest.Attempt, latest.LeaseVersion, lastError)
			}
			time.Sleep(50 * time.Millisecond)
		}
		if latest == nil {
			t.Fatalf("same-case task %s was never observed", taskID)
		}
		t.Fatalf("same-case task %s remained %s after 60 seconds", taskID, latest.Status)
		return nil
	}
	completedOne := waitSuccess(taskOne)
	completedTwo := waitSuccess(taskTwo)
	if completedOne.Attempt != 1 || completedTwo.Attempt != 1 || completedOne.LeaseVersion != 1 || completedTwo.LeaseVersion != 1 {
		t.Fatalf("same-case tasks should each complete on generation 1: task1 attempt=%d generation=%d task2 attempt=%d generation=%d", completedOne.Attempt, completedOne.LeaseVersion, completedTwo.Attempt, completedTwo.LeaseVersion)
	}
	var ownerOne, ownerTwo string
	if err := db.QueryRowContext(ctx, `SELECT worker_id FROM task_events WHERE task_id=$1 AND event_type='claimed' ORDER BY id LIMIT 1`, taskOne).Scan(&ownerOne); err != nil {
		t.Fatalf("read first same-case claim: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT worker_id FROM task_events WHERE task_id=$1 AND event_type='claimed' ORDER BY id LIMIT 1`, taskTwo).Scan(&ownerTwo); err != nil {
		t.Fatalf("read second same-case claim: %v", err)
	}
	if ownerOne != "e2e-worker-a" || ownerTwo != "e2e-worker-b" {
		t.Fatalf("same-case tasks were not claimed by the orchestrated separate containers: owner1=%s owner2=%s", ownerOne, ownerTwo)
	}
	if completedOne.Result == nil || completedTwo.Result == nil || completedOne.Result.Artifacts["artifact_sha256"] == "" || completedOne.Result.Artifacts["artifact_sha256"] != completedTwo.Result.Artifacts["artifact_sha256"] {
		t.Fatalf("same-case task DB results disagree: result1=%+v result2=%+v", completedOne.Result, completedTwo.Result)
	}
	data, info, err := store.Get(ctx, artifactKey)
	if err != nil || len(data) == 0 || info.SHA256 != completedOne.Result.Artifacts["artifact_sha256"] {
		t.Fatalf("independent MinIO object validation failed: size=%d info_sha=%s task_sha=%s err=%v", len(data), info.SHA256, completedOne.Result.Artifacts["artifact_sha256"], err)
	}
	t.Logf("PASS: two Worker containers reached the same-key PUT barrier before MinIO accepted either; owners=%s/%s; both task results reference identical verified SHA256=%s", ownerOne, ownerTwo, info.SHA256)
}

func waitArtifactInfo(ctx context.Context, store artifacts.Store, key string, timeout time.Duration) ([]byte, artifacts.ObjectInfo, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		data, info, err := store.Get(ctx, key)
		if err == nil {
			return data, info, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, artifacts.ObjectInfo{}, ctx.Err()
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, artifacts.ObjectInfo{}, fmt.Errorf("object did not appear within %s: %w", timeout, lastErr)
}

func waitRetryEvent(t *testing.T, ctx context.Context, db *sql.DB, taskID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM task_events WHERE task_id=$1 AND event_type='retried'`, taskID).Scan(&count); err != nil {
			t.Fatalf("read response-loss retry event: %v", err)
		}
		if count == 1 {
			return
		}
		if count > 1 {
			t.Fatalf("response-loss task has %d retried events before recovery", count)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for PostgreSQL to record the Worker response-loss retry")
}
