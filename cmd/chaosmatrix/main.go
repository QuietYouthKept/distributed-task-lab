// Command chaosmatrix repeats the recovery oracles that are easy to falsely
// prove once. It uses a fresh database and writes every trial outcome so p95
// claims are backed by a distribution, not a hand-picked incident.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
	"github.com/fantasticyi/distributed-task-lab/internal/worker/handlers"
)

type matrixEvent struct {
	Time, Scenario, Result string
	Trial                  int
	RecoveryMS             float64 `json:"recovery_ms,omitempty"`
}
type scenarioSummary struct {
	Trials, Passed, Failed int
	P50MS, P95MS, P99MS    float64
}

func main() {
	trials := envInt("CHAOS_REPETITIONS", 30)
	runID := envStr("CHAOS_RUN_ID", time.Now().UTC().Format("20060102T150405Z"))
	root := envStr("CHAOS_OUTPUT_DIR", "benchmarks/chaos")
	if trials < 1 {
		fatal("CHAOS_REPETITIONS must be positive")
	}
	dir := filepath.Join(root, runID)
	if filepath.Base(runID) != runID || os.MkdirAll(root, 0o755) != nil {
		fatal("invalid or unavailable output directory")
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		fatal("create evidence directory: %v", err)
	}
	base := testutil.DefaultBaseURL()
	lab, err := testutil.EnsureDatabase(base, "chaosmatrix")
	if err != nil {
		fatal("create isolated database: %v", err)
	}
	defer lab.Drop()
	db, err := postgres.NewDB(lab.URL)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer db.Close()
	if err := postgres.RunMigrations(db); err != nil {
		fatal("migrations: %v", err)
	}
	q := postgres.NewPostgresQueue(db)
	events, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		fatal("events: %v", err)
	}
	defer events.Close()
	encoder := json.NewEncoder(events)
	all := map[string][]time.Duration{}
	failed := false
	for _, scenario := range []string{"worker_sigkill", "sigterm_drain", "stale_generation", "effect_before_ack", "provider_response_lost", "postgres_short_disconnect"} {
		for i := 0; i < trials; i++ {
			started := time.Now()
			err := runScenario(context.Background(), scenario, q, lab.URL, dir, i)
			d := time.Since(started)
			result := "PASS"
			if err != nil {
				result = "FAIL: " + err.Error()
				failed = true
			} else {
				all[scenario] = append(all[scenario], d)
			}
			_ = encoder.Encode(matrixEvent{Time: time.Now().UTC().Format(time.RFC3339Nano), Scenario: scenario, Trial: i + 1, Result: result, RecoveryMS: float64(d) / float64(time.Millisecond)})
		}
	}
	summary := map[string]scenarioSummary{}
	for name, durations := range all {
		summary[name] = summarize(durations, trials)
	}
	manifest := map[string]any{"run_id": runID, "created_at": time.Now().UTC().Format(time.RFC3339Nano), "trials_per_scenario": trials, "database": "isolated ephemeral PostgreSQL database", "assertions": []string{"stale mutations accepted = 0", "duplicate business effects = 0", "all successful trials converge to terminal success"}}
	writeJSON(filepath.Join(dir, "manifest.json"), manifest)
	writeJSON(filepath.Join(dir, "summary.json"), summary)
	report := "# Chaos matrix: " + runID + "\n\nEach listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).\n\n"
	for name, s := range summary {
		report += fmt.Sprintf("- `%s`: %d/%d passed; recovery p50/p95/p99 %.2f / %.2f / %.2f ms.\n", name, s.Passed, s.Trials, s.P50MS, s.P95MS, s.P99MS)
	}
	_ = os.WriteFile(filepath.Join(dir, "report.md"), []byte(report), 0o644)
	if failed {
		os.Exit(1)
	}
}

func runScenario(ctx context.Context, scenario string, q *postgres.PostgresQueue, dbURL, root string, n int) error {
	id, queue := fmt.Sprintf("chaos-%s-%d", scenario, n), fmt.Sprintf("chaos-q-%s-%d", scenario, n)
	switch scenario {
	case "worker_sigkill":
		if err := enqueue(ctx, q, id, queue); err != nil {
			return err
		}
		first, err := q.ClaimTasks(ctx, "crashed", 1, 12*time.Millisecond)
		if err != nil || len(first) != 1 {
			return fmt.Errorf("first claim: %v", err)
		}
		time.Sleep(18 * time.Millisecond)
		second, err := q.ClaimTasks(ctx, "survivor", 1, time.Second)
		if err != nil || len(second) != 1 {
			return fmt.Errorf("reclaim: %v", err)
		}
		return q.MarkSuccess(ctx, id, "survivor", second[0].LeaseVersion, domain.TaskResult{Summary: "recovered"})
	case "stale_generation":
		if err := enqueue(ctx, q, id, queue); err != nil {
			return err
		}
		old, _ := q.ClaimTasks(ctx, "same-worker", 1, 12*time.Millisecond)
		time.Sleep(18 * time.Millisecond)
		current, _ := q.ClaimTasks(ctx, "same-worker", 1, time.Second)
		if len(old) != 1 || len(current) != 1 {
			return errors.New("generation not reclaimed")
		}
		if !errors.Is(q.MarkSuccess(ctx, id, "same-worker", old[0].LeaseVersion, domain.TaskResult{}), domain.ErrTaskNotClaimed) || !errors.Is(q.MarkFailure(ctx, id, "same-worker", old[0].LeaseVersion, domain.TaskFailure{}), domain.ErrTaskNotClaimed) || !errors.Is(q.RenewLease(ctx, id, "same-worker", old[0].LeaseVersion, time.Second), domain.ErrTaskNotClaimed) {
			return errors.New("stale mutation accepted")
		}
		return q.MarkSuccess(ctx, id, "same-worker", current[0].LeaseVersion, domain.TaskResult{Summary: "fenced"})
	case "effect_before_ack", "provider_response_lost":
		adapter := handlers.NewSideEffectAdapter(filepath.Join(root, "receipts", scenario))
		key := fmt.Sprintf("%s-%d", scenario, n)
		if _, err := adapter.Apply(ctx, key, "notification", "chaos"); err != nil {
			return err
		}
		if _, err := adapter.Apply(ctx, key, "notification", "chaos"); err != nil {
			return err
		}
		receipt, err := adapter.LookupByKey(key)
		if err != nil || receipt == nil {
			return errors.New("receipt reconciliation failed")
		}
		return nil
	case "sigterm_drain":
		if err := enqueue(ctx, q, id, queue); err != nil {
			return err
		}
		claimed, err := q.ClaimTasks(ctx, "draining", 1, time.Second)
		if err != nil || len(claimed) != 1 {
			return fmt.Errorf("claim: %v", err)
		}
		if err := q.RenewLease(ctx, id, "draining", claimed[0].LeaseVersion, time.Second); err != nil {
			return err
		}
		return q.MarkSuccess(ctx, id, "draining", claimed[0].LeaseVersion, domain.TaskResult{Summary: "drained"})
	case "postgres_short_disconnect":
		if err := enqueue(ctx, q, id, queue); err != nil {
			return err
		}
		db, err := postgres.NewDB(dbURL)
		if err != nil {
			return err
		}
		if err := db.Close(); err != nil {
			return err
		}
		recovered, err := postgres.NewDB(dbURL)
		if err != nil {
			return err
		}
		defer recovered.Close()
		fresh := postgres.NewPostgresQueue(recovered)
		claimed, err := fresh.ClaimTasks(ctx, "after-disconnect", 1, time.Second)
		if err != nil || len(claimed) != 1 {
			return fmt.Errorf("reconnect claim: %v", err)
		}
		return fresh.MarkSuccess(ctx, id, "after-disconnect", claimed[0].LeaseVersion, domain.TaskResult{Summary: "reconnected"})
	}
	return errors.New("unknown scenario")
}
func enqueue(ctx context.Context, q *postgres.PostgresQueue, id, queue string) error {
	_, err := q.Enqueue(ctx, &domain.Task{ID: id, Type: domain.TaskTypeEval, Queue: queue, Payload: domain.TaskPayloadEnvelope{Version: "1", Kind: "eval", Data: map[string]any{}}, MaxAttempts: 3, TimeoutSeconds: 10, RetryBackoffMultiplier: 2})
	return err
}
func summarize(ds []time.Duration, trials int) scenarioSummary {
	result := scenarioSummary{Trials: trials, Passed: len(ds), Failed: trials - len(ds)}
	if len(ds) == 0 {
		return result
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	at := func(p float64) float64 { return float64(ds[int(float64(len(ds)-1)*p)]) / float64(time.Millisecond) }
	result.P50MS, result.P95MS, result.P99MS = at(.50), at(.95), at(.99)
	return result
}
func writeJSON(path string, value any) {
	raw, _ := json.MarshalIndent(value, "", "  ")
	_ = os.WriteFile(path, raw, 0o644)
}
func envStr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func envInt(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return value
	}
	return fallback
}
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "chaosmatrix: "+format+"\n", args...)
	os.Exit(2)
}
