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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

const (
	apiToken   = "wave-two-e2e-operator-token"
	adminToken = "wave-two-e2e-admin-token"
)

type appProcess struct {
	command *exec.Cmd
	done    chan error
}

func TestAPITwoWorkersRecoverSharedArtifactAfterAckProcessCrash(t *testing.T) {
	if os.Getenv("REQUIRE_TEST_DATABASE") != "1" {
		t.Skip("REQUIRE_TEST_DATABASE=1 is required for process-level PostgreSQL recovery test")
	}
	if os.Getenv("REQUIRE_MINIO") != "1" {
		t.Skip("REQUIRE_MINIO=1 is required for process-level shared-artifact recovery test")
	}
	endpoint, bucket := os.Getenv("MINIO_ENDPOINT"), os.Getenv("MINIO_BUCKET")
	accessKey, secretKey := os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")
	if endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Fatal("required MinIO environment variable names are not fully configured")
	}

	baseURL := testutil.DefaultBaseURL()
	database, err := testutil.EnsureDatabase(baseURL, "tasklab_process_e2e")
	if err != nil {
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(database.Drop)
	db, err := postgres.NewDB(database.URL)
	if err != nil {
		t.Fatalf("connect isolated database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	prefix := "process-e2e/" + uuid.NewString()
	store, err := artifacts.NewS3Store(artifacts.S3Config{
		Endpoint: endpoint, Bucket: bucket, Prefix: prefix, AccessKey: accessKey, SecretKey: secretKey,
		Region: "us-east-1", MaxBytes: 64 << 20, AllowHTTP: strings.HasPrefix(endpoint, "http://"),
	})
	if err != nil {
		t.Fatalf("configure S3 test store: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("verify S3 test bucket: %v", err)
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	apiBinary := buildBinary(t, root, binDir, "api", "./cmd/api")
	workerBinary := buildBinary(t, root, binDir, "worker", "./cmd/worker")
	apiPort := freePort(t)
	workerPortA := freePort(t, apiPort)
	workerPortB := freePort(t, apiPort, workerPortA)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	apiEnv := map[string]string{
		"APP_PROFILE": "development", "DATABASE_URL": database.URL, "API_PORT": fmt.Sprint(apiPort),
		"API_TOKEN": apiToken, "ADMIN_TOKEN": adminToken, "LOG_LEVEL": "error",
	}
	apiProc := startProcess(t, apiBinary, root, filepath.Join(binDir, "api.log"), apiEnv)
	defer apiProc.stop()
	waitHTTP(t, apiProc, apiURL+"/ready", 20*time.Second)
	workerDatabaseURLs := provisionFencedWorkerLogins(t, db, database.URL, []string{"e2e-worker-a", "e2e-worker-b"})

	commonWorkerEnv := map[string]string{
		"APP_PROFILE":            "development",
		"AUTO_MIGRATE":           "false",
		"ARTIFACT_STORE_BACKEND": "s3", "ARTIFACT_S3_ENDPOINT": endpoint,
		"ARTIFACT_S3_BUCKET": bucket, "ARTIFACT_S3_PREFIX": prefix,
		"ARTIFACT_S3_ACCESS_KEY": accessKey, "ARTIFACT_S3_SECRET_KEY": secretKey,
		"ARTIFACT_S3_REGION": "us-east-1", "ARTIFACT_ALLOW_HTTP": fmt.Sprint(strings.HasPrefix(endpoint, "http://")),
		"ARTIFACT_ALLOW_BUCKET_CREATE": "false", "WORKER_CONCURRENCY": "1",
		"WORKER_POLL_INTERVAL": "20ms", "CLAIM_BATCH_SIZE": "1", "LEASE_TIMEOUT": "4s",
		"DB_OPERATION_TIMEOUT": "1s", "SHUTDOWN_TIMEOUT": "1s", "LOG_LEVEL": "error",
	}
	workerEnvA := cloneEnv(commonWorkerEnv)
	workerEnvA["WORKER_ID"], workerEnvA["WORKER_HEALTH_PORT"] = "e2e-worker-a", fmt.Sprint(workerPortA)
	workerEnvA["DATABASE_URL"] = workerDatabaseURLs["e2e-worker-a"]
	workerA := startProcess(t, workerBinary, root, filepath.Join(binDir, "worker-a.log"), workerEnvA)
	defer workerA.stop()
	waitHTTP(t, workerA, fmt.Sprintf("http://127.0.0.1:%d/ready", workerPortA), 20*time.Second)
	workerEnvB := cloneEnv(commonWorkerEnv)
	workerEnvB["WORKER_ID"], workerEnvB["WORKER_HEALTH_PORT"] = "e2e-worker-b", fmt.Sprint(workerPortB)
	workerEnvB["DATABASE_URL"] = workerDatabaseURLs["e2e-worker-b"]
	workerB := startProcess(t, workerBinary, root, filepath.Join(binDir, "worker-b.log"), workerEnvB)
	defer workerB.stop()
	waitHTTP(t, workerB, fmt.Sprintf("http://127.0.0.1:%d/ready", workerPortB), 20*time.Second)

	runID := "process-recovery-" + uuid.NewString()
	const cases = 10_000_000
	request := map[string]any{
		"type": "eval", "payload": map[string]any{
			"version": "1", "kind": "eval", "data": map[string]any{
				"execution_mode": "deterministic", "dataset_version": "e2e-dataset-v1",
				"model_profile": "deterministic-local", "prompt_version": "e2e-prompt-v1",
				"run_id": runID, "case_start": 0, "case_end": cases,
			},
		}, "max_attempts": 3, "timeout_seconds": 120,
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
	response, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create task through API: %v", err)
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&created)
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusCreated || decodeErr != nil || closeErr != nil || created.ID == "" {
		t.Fatalf("create task response status=%d, id_present=%t, decode_err=%v, close_err=%v", response.StatusCode, created.ID != "", decodeErr, closeErr)
	}

	tx, owner := lockClaimedTask(t, ctx, db, created.ID, 30*time.Second)
	defer func() { _ = tx.Rollback() }()
	claimed := getTask(t, ctx, apiURL, created.ID)
	if claimed.Status != domain.TaskStatusRunning || claimed.LeasedBy == nil || *claimed.LeasedBy != owner || claimed.LeaseVersion < 1 {
		t.Fatalf("API task state does not match claimed owner: status=%s owner=%v expected=%s", claimed.Status, claimed.LeasedBy, owner)
	}
	originalLeaseVersion := claimed.LeaseVersion

	identity := fmt.Sprintf("e2e-dataset-v1|deterministic-local|e2e-prompt-v1|%s|0|%d", runID, cases)
	fingerprint := sha256.Sum256([]byte(identity))
	artifactKey := fmt.Sprintf("eval/%s/%s.json", runID, hex.EncodeToString(fingerprint[:]))
	if _, err := waitArtifact(ctx, store, artifactKey, 30*time.Second); err != nil {
		t.Fatalf("wait for shared artifact before ACK: %v", err)
	}
	stillRunning := getTask(t, ctx, apiURL, created.ID)
	if stillRunning.Status != domain.TaskStatusRunning {
		t.Fatalf("task status after artifact Put and before releasing ACK barrier=%s, want running", stillRunning.Status)
	}
	if owner == "e2e-worker-a" {
		workerA.kill(t)
	} else {
		workerB.kill(t)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("release ACK barrier after terminating owner %s: %v", owner, err)
	}

	completed := waitTaskState(t, ctx, apiURL, created.ID, domain.TaskStatusSuccess, 30*time.Second)
	if completed.Attempt < 2 || completed.LeaseVersion < 2 {
		t.Fatalf("reclaimed task attempt=%d lease_version=%d, want both >=2", completed.Attempt, completed.LeaseVersion)
	}
	if completed.Result == nil || completed.Result.Artifacts["idempotent_reused"] != "true" {
		t.Fatalf("recovery result did not reuse the pre-crash shared artifact: %+v", completed.Result)
	}
	artifact, _, err := store.Get(ctx, artifactKey)
	if err != nil {
		t.Fatalf("read recovered artifact: %v", err)
	}
	var stored struct {
		RunID       string `json:"RunID"`
		Fingerprint string `json:"Fingerprint"`
		CaseEnd     int    `json:"CaseEnd"`
	}
	if err := json.Unmarshal(artifact, &stored); err != nil {
		t.Fatalf("decode recovered artifact: %v", err)
	}
	if stored.RunID != runID || stored.Fingerprint != hex.EncodeToString(fingerprint[:]) || stored.CaseEnd != cases {
		t.Fatalf("recovered artifact identity mismatch: run_id=%q case_end=%d", stored.RunID, stored.CaseEnd)
	}
	staleErr := postgres.NewPostgresQueue(db).MarkSuccess(ctx, created.ID, owner, originalLeaseVersion, domain.TaskResult{Summary: "stale owner must be fenced"})
	if !errors.Is(staleErr, domain.ErrTaskNotClaimed) {
		t.Fatalf("stale owner generation write returned %v, want ErrTaskNotClaimed", staleErr)
	}
	t.Logf("PASS: API-created task recovered by the surviving worker after owner %s was killed after shared Put and before ACK; attempt=%d lease_version=%d; stale generation %d rejected", owner, completed.Attempt, completed.LeaseVersion, originalLeaseVersion)
}

func provisionFencedWorkerLogins(t *testing.T, adminDB *sql.DB, databaseURL string, workerIDs []string) map[string]string {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	var roleNames []string
	urls := make(map[string]string, len(workerIDs))
	for index, workerID := range workerIDs {
		roleName := fmt.Sprintf("tasklab_e2e_%s_%d", suffix, index)
		password := uuid.NewString() + uuid.NewString()
		if _, err := adminDB.Exec(`CREATE ROLE ` + postgresQuoteIdentifier(roleName) + ` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD '` + password + `'`); err != nil {
			t.Fatalf("create isolated process-test Worker login: %v", err)
		}
		roleNames = append(roleNames, roleName)
		for _, statement := range []string{
			`GRANT CONNECT ON DATABASE ` + postgresQuoteIdentifier(databaseNameFromURL(t, databaseURL)) + ` TO ` + postgresQuoteIdentifier(roleName),
			`GRANT USAGE ON SCHEMA public TO ` + postgresQuoteIdentifier(roleName),
			`GRANT SELECT ON TABLE tasks, queues, schema_migrations TO ` + postgresQuoteIdentifier(roleName),
			`GRANT INSERT ON TABLE workers TO ` + postgresQuoteIdentifier(roleName),
			`GRANT SELECT ON TABLE task_events TO ` + postgresQuoteIdentifier(roleName),
			`GRANT UPDATE ON TABLE workers TO ` + postgresQuoteIdentifier(roleName),
			`GRANT SELECT, INSERT, UPDATE ON TABLE effects TO ` + postgresQuoteIdentifier(roleName),
			`GRANT UPDATE (rate_tokens, rate_updated_at) ON TABLE queues TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_identity() TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_terminalize_exhausted_leases() TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_claim_task(TEXT, BIGINT) TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_ack(TEXT, INTEGER, JSONB) TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_fail(TEXT, INTEGER, TEXT, TEXT, BOOLEAN) TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_renew(TEXT, INTEGER, BIGINT) TO ` + postgresQuoteIdentifier(roleName),
			`GRANT EXECUTE ON FUNCTION tasklab_worker_insert_event(TEXT, TEXT, TEXT, JSONB) TO ` + postgresQuoteIdentifier(roleName),
		} {
			if _, err := adminDB.Exec(statement); err != nil {
				t.Fatalf("grant isolated process-test Worker access: %v", err)
			}
		}
		if _, err := adminDB.Exec(`INSERT INTO worker_db_identities(login_role, worker_id) VALUES ($1, $2)`, roleName, workerID); err != nil {
			t.Fatalf("register process-test Worker database identity: %v", err)
		}
		parsed, err := url.Parse(databaseURL)
		if err != nil {
			t.Fatalf("parse process-test database URL: %v", err)
		}
		parsed.User = url.UserPassword(roleName, password)
		urls[workerID] = parsed.String()
	}
	t.Cleanup(func() {
		for _, roleName := range roleNames {
			_, _ = adminDB.Exec(`DROP OWNED BY ` + postgresQuoteIdentifier(roleName))
			_, _ = adminDB.Exec(`DROP ROLE IF EXISTS ` + postgresQuoteIdentifier(roleName))
		}
	})
	return urls
}

func postgresQuoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func databaseNameFromURL(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if name == "" || strings.Contains(name, "/") {
		t.Fatalf("test database URL has invalid database path")
	}
	return name
}

func buildBinary(t *testing.T, root, directory, name, packagePath string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	command := exec.Command("go", "build", "-o", path, packagePath)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s process binary (output bytes=%d): %v", name, len(output), err)
	}
	return path
}

func startProcess(t *testing.T, binary, directory, logPath string, overrides map[string]string) *appProcess {
	t.Helper()
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create private process log: %v", err)
	}
	command := exec.Command(binary)
	command.Dir = directory
	command.Env = mergedEnv(overrides)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start process: %v", err)
	}
	process := &appProcess{command: command, done: make(chan error, 1)}
	go func() {
		process.done <- command.Wait()
		_ = logFile.Close()
	}()
	t.Cleanup(func() { process.stop() })
	return process
}

func (p *appProcess) kill(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		return
	default:
	}
	if err := p.command.Process.Kill(); err != nil {
		t.Fatalf("terminate worker before ACK: %v", err)
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker process did not terminate within 5 seconds")
	}
}

func (p *appProcess) stop() {
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.command.Process.Kill()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
}

func mergedEnv(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, value := range os.Environ() {
		if key, content, ok := strings.Cut(value, "="); ok {
			values[key] = content
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func freePort(t *testing.T, excluded ...int) int {
	t.Helper()
	for attempt := 0; attempt < 20; attempt++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve temporary port: %v", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		available := true
		for _, reserved := range excluded {
			if port == reserved {
				available = false
				break
			}
		}
		if available {
			return port
		}
	}
	t.Fatal("could not allocate distinct temporary listener ports")
	return 0
}

func waitHTTP(t *testing.T, process *appProcess, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		select {
		case err := <-process.done:
			t.Fatalf("process exited before health became ready: %v", err)
		default:
		}
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("process health endpoint did not become ready within %s", timeout)
}

func waitTaskState(t *testing.T, ctx context.Context, apiURL, id string, want domain.TaskStatus, timeout time.Duration) *domain.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var latest *domain.Task
	for time.Now().Before(deadline) {
		latest = getTask(t, ctx, apiURL, id)
		if latest.Status == want {
			return latest
		}
		if latest.Status == domain.TaskStatusFailed || latest.Status == domain.TaskStatusDLQ {
			t.Fatalf("task reached terminal state %s while waiting for %s", latest.Status, want)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if latest == nil {
		t.Fatalf("task never became %s", want)
	}
	t.Fatalf("task state deadline: got %s, want %s", latest.Status, want)
	return nil
}

func getTask(t *testing.T, ctx context.Context, apiURL, id string) *domain.Task {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/v1/tasks/"+id, nil)
	if err != nil {
		t.Fatalf("build task read request: %v", err)
	}
	token := apiToken
	if containerToken := os.Getenv("E2E_API_TOKEN"); containerToken != "" {
		token = containerToken
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("read task through API: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read task through API returned HTTP %d", response.StatusCode)
	}
	var task domain.Task
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatalf("decode task response: %v", err)
	}
	return &task
}

func waitArtifact(ctx context.Context, store artifacts.Store, key string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		data, _, err := store.Get(ctx, key)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("artifact did not appear within %s: %w", timeout, lastErr)
}

func lockClaimedTask(t *testing.T, ctx context.Context, db *sql.DB, taskID string, timeout time.Duration) (*sql.Tx, string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin task ACK barrier transaction: %v", err)
		}
		var owner string
		err = tx.QueryRowContext(ctx, `SELECT leased_by FROM tasks WHERE id=$1 AND status='running' FOR UPDATE`, taskID).Scan(&owner)
		if err == nil {
			if owner != "e2e-worker-a" && owner != "e2e-worker-b" {
				_ = tx.Rollback()
				t.Fatalf("task leased by unexpected worker %q", owner)
			}
			return tx, owner
		}
		_ = tx.Rollback()
		if err != sql.ErrNoRows {
			t.Fatalf("lock claimed task before ACK: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no worker claimed the API-created task within the ACK barrier deadline")
	return nil, ""
}

func cloneEnv(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
