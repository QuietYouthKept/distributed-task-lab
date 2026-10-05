package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type loadgenArtifactSummary struct {
	RunID, Profile                                                     string
	Tasks, Producers, Workers, Concurrency, Queues, PayloadBytes       int
	SubmitSeconds, ProcessingSeconds, Throughput                       float64
	Success, DLQ, SubmissionErrors, Retries, Reclaims, DuplicateClaims int64
	ClaimP50MS, ClaimP95MS, ClaimP99MS                                 float64
	QueueWaitP50MS, QueueWaitP95MS, QueueWaitP99MS                     float64
	EndToEndP50MS, EndToEndP95MS, EndToEndP99MS                        float64
}

type loadgenRawSample struct {
	Metric, TaskID string
	ValueMS        float64
}

// writeLoadgenArtifacts creates a portable evidence bundle. It deliberately
// stores raw claim samples as well as the summary: a reported percentile can be
// independently recomputed, and failed/correctness-invalid runs remain useful.
func writeLoadgenArtifacts(root, runID string, db *sql.DB, s loadgenArtifactSummary, samples []loadgenRawSample) (string, error) {
	if filepath.Base(runID) != runID || runID == "." || runID == "" {
		return "", fmt.Errorf("invalid run ID %q", runID)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	dir := filepath.Join(root, runID)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", err
	}
	gitSHA := "unknown"
	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		gitSHA = strings.TrimSpace(string(out))
	}
	pgVersion := "unknown"
	_ = db.QueryRow("SHOW server_version").Scan(&pgVersion)
	manifest := map[string]any{
		"run_id": runID, "created_at": time.Now().UTC().Format(time.RFC3339Nano), "git_sha": gitSHA,
		"go_version": runtime.Version(), "postgres_version": pgVersion,
		"machine":    map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "cpu_count": runtime.NumCPU()},
		"parameters": map[string]any{"profile": s.Profile, "tasks": s.Tasks, "producers": s.Producers, "workers": s.Workers, "worker_concurrency": s.Concurrency, "queues": s.Queues, "payload_bytes": s.PayloadBytes},
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return "", err
	}
	f, err := os.Create(filepath.Join(dir, "raw.csv"))
	if err != nil {
		return "", err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"metric", "task_id", "value_ms"})
	for _, sample := range samples {
		_ = w.Write([]string{sample.Metric, sample.TaskID, strconv.FormatFloat(sample.ValueMS, 'f', 3, 64)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	var dbStats map[string]any = map[string]any{"postgres_version": pgVersion}
	var commits, rollbacks, deadlocks int64
	if err := db.QueryRow("SELECT xact_commit, xact_rollback, deadlocks FROM pg_stat_database WHERE datname = current_database()").Scan(&commits, &rollbacks, &deadlocks); err == nil {
		dbStats["xact_commit"] = commits
		dbStats["xact_rollback"] = rollbacks
		dbStats["deadlocks"] = deadlocks
	}
	if err := writeJSON(filepath.Join(dir, "postgres-stats.json"), dbStats); err != nil {
		return "", err
	}
	if err := writeJSON(filepath.Join(dir, "summary.json"), s); err != nil {
		return "", err
	}
	report := fmt.Sprintf("# Loadgen report: %s\n\n- Profile: `%s`; tasks: %d; workers: %d x %d; queues: %d; payload: %d bytes.\n- Throughput: %.2f tasks/s; processing: %.3fs; submission: %.3fs.\n- Claim latency: p50 %.3fms, p95 %.3fms, p99 %.3fms.\n- Queue wait: p50 %.3fms, p95 %.3fms, p99 %.3fms.\n- End-to-end completion: p50 %.3fms, p95 %.3fms, p99 %.3fms.\n- Correctness: success=%d, DLQ=%d, retries=%d, reclaims=%d, duplicate claims=%d, submission errors=%d.\n\nThis is one point in a curve, not a capacity claim. Compare matrix runs only when their manifest parameters and PostgreSQL durability settings match. `raw.csv` contains every claim, queue-wait, and end-to-end sample.\n", runID, s.Profile, s.Tasks, s.Workers, s.Concurrency, s.Queues, s.PayloadBytes, s.Throughput, s.ProcessingSeconds, s.SubmitSeconds, s.ClaimP50MS, s.ClaimP95MS, s.ClaimP99MS, s.QueueWaitP50MS, s.QueueWaitP95MS, s.QueueWaitP99MS, s.EndToEndP50MS, s.EndToEndP95MS, s.EndToEndP99MS, s.Success, s.DLQ, s.Retries, s.Reclaims, s.DuplicateClaims, s.SubmissionErrors)
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(report), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
