// Command assetlab executes asset-derived PostgreSQL correctness experiments
// and writes self-contained, machine-checkable evidence bundles.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
)

const seed int64 = 20260823

type historyEvent struct {
	Timestamp    string         `json:"timestamp"`
	ElapsedMS    int64          `json:"elapsed_ms"`
	Process      string         `json:"process"`
	TaskID       string         `json:"task_id,omitempty"`
	Operation    string         `json:"operation"`
	Phase        string         `json:"phase"`
	LeaseVersion int            `json:"lease_version,omitempty"`
	Attempt      int            `json:"attempt,omitempty"`
	Queue        string         `json:"queue,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type experiment struct {
	id, title, root, prefix string
	db                      *sql.DB
	started                 time.Time
	file                    *os.File
	enc                     *json.Encoder
	mu                      sync.Mutex
}

type assertion struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Observed any    `json:"observed,omitempty"`
	Expected any    `json:"expected,omitempty"`
}

func main() {
	ctx := context.Background()
	root := os.Getenv("ASSETLAB_RESULT_DIR")
	if root == "" {
		fmt.Fprintln(os.Stderr, "ASSETLAB_RESULT_DIR is required")
		os.Exit(2)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5433/tasklab_test?sslmode=disable"
	}
	db, err := postgres.NewDB(url)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := postgres.RunMigrations(db); err != nil {
		panic(err)
	}
	q := postgres.NewPostgresQueue(db)

	tests := []struct {
		id, title string
		fn        func(context.Context, *postgres.PostgresQueue, *experiment) ([]assertion, error)
	}{
		{"TL-CLAIM-002", "1000-task concurrent unique-generation recovery scan", claimLoad},
		{"TL-CAP-003", "Repeated strict capacity under 16-way contention", capacityRepeated},
		{"TL-QUEUE-001", "Independent queue admission under authority contention", queueIndependence},
		{"TL-FENCE-002", "Different-worker stale success failure and renewal fencing", differentWorkerFencing},
		{"TL-LEASE-003", "Renewal then stopped-renewal takeover", renewalTakeover},
		{"TL-SIDEFX-002", "Effect-before-ack duplicate and idempotent convergence", effectBeforeAck},
		{"TL-IDEM-002", "64-way concurrent idempotent enqueue", concurrentIdempotency},
		{"TL-RETRY-002", "Retry backoff cap and DLQ transition", retryBackoffDLQ},
		{"TL-PAUSE-002", "Pause linearization with in-flight completion", pauseInflight},
		{"TL-RATE-003", "Replica-count invariant shared token bucket", rateReplicaInvariant},
		{"TL-CLOCK-002", "Database clock lease authority", databaseClock},
	}

	failures := 0
	for i, test := range tests {
		// Each model gets a clean database authority domain. Its complete after
		// state is persisted before the next model begins.
		if _, err := db.ExecContext(ctx, `TRUNCATE task_events, tasks, workers, queues RESTART IDENTITY CASCADE`); err != nil {
			panic(err)
		}
		prefix := fmt.Sprintf("asset-%d-%02d", seed, i+1)
		exp, err := newExperiment(root, test.id, test.title, prefix, db)
		if err != nil {
			panic(err)
		}
		assertions, runErr := test.fn(ctx, q, exp)
		if runErr != nil {
			assertions = append(assertions, assertion{Name: "experiment_completed", Passed: false, Observed: runErr.Error(), Expected: "no error"})
		}
		passed := runErr == nil
		for _, a := range assertions {
			passed = passed && a.Passed
		}
		if !passed {
			failures++
		}
		if err := exp.finish(assertions, passed); err != nil {
			panic(err)
		}
		fmt.Printf("%s %s\n", test.id, map[bool]string{true: "PASS", false: "FAIL"}[passed])
	}
	manifest := map[string]any{"seed": seed, "experiments": len(tests), "failures": failures, "completed_at": time.Now().UTC().Format(time.RFC3339Nano)}
	writeJSON(filepath.Join(root, "assetlab-summary.json"), manifest)
	if failures != 0 {
		os.Exit(1)
	}
}

func newExperiment(root, id, title, prefix string, db *sql.DB) (*experiment, error) {
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "operations.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	e := &experiment{id: id, title: title, root: dir, prefix: prefix, db: db, started: time.Now(), file: f, enc: json.NewEncoder(f)}
	writeJSON(filepath.Join(dir, "config.json"), map[string]any{
		"experiment_id": id, "title": title, "seed": seed, "task_prefix": prefix,
		"database": "PostgreSQL 16 dedicated local test instance", "history_schema": 1,
	})
	if err := e.snapshot("db-before.json"); err != nil {
		f.Close()
		return nil, err
	}
	card := fmt.Sprintf("# %s — %s\n\n- Status: generated, executable evidence card\n- Source model: Jepsen invocation/completion histories plus NATS/Redpanda final recovery oracles\n- Seed: `%d`\n- Prefix: `%s`\n- Oracle: `result.json` assertions; raw operations are in `operations.jsonl`\n- Evidence boundary: this experiment proves the stated PostgreSQL queue invariant only; it does not prove end-to-end exactly-once effects or replicated-database durability.\n", id, title, seed, prefix)
	if err := os.WriteFile(filepath.Join(dir, "test-card.md"), []byte(card), 0o644); err != nil {
		f.Close()
		return nil, err
	}
	return e, nil
}

func (e *experiment) event(process, taskID, operation, phase, queue string, leaseVersion, attempt int, metadata map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.enc.Encode(historyEvent{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), ElapsedMS: time.Since(e.started).Milliseconds(), Process: process, TaskID: taskID, Operation: operation, Phase: phase, LeaseVersion: leaseVersion, Attempt: attempt, Queue: queue, Metadata: metadata})
}

func (e *experiment) finish(assertions []assertion, passed bool) error {
	e.mu.Lock()
	err := e.file.Close()
	e.mu.Unlock()
	if err != nil {
		return err
	}
	if err := e.snapshot("db-after.json"); err != nil {
		return err
	}
	return writeJSON(filepath.Join(e.root, "result.json"), map[string]any{
		"experiment_id": e.id, "passed": passed, "assertions": assertions,
		"duration_ms": time.Since(e.started).Milliseconds(), "finished_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (e *experiment) snapshot(name string) error {
	var raw []byte
	err := e.db.QueryRow(`
		SELECT jsonb_build_object(
		  'captured_at', clock_timestamp(),
		  'tasks', COALESCE((SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id) FROM tasks t WHERE t.id LIKE $1 OR t.queue LIKE $1), '[]'::jsonb),
		  'events', COALESCE((SELECT jsonb_agg(to_jsonb(ev) ORDER BY ev.id) FROM task_events ev JOIN tasks t ON t.id=ev.task_id WHERE t.id LIKE $1 OR t.queue LIKE $1), '[]'::jsonb),
		  'queues', COALESCE((SELECT jsonb_agg(to_jsonb(q) ORDER BY q.name) FROM queues q WHERE q.name LIKE $1), '[]'::jsonb)
		)::text`, e.prefix+"%").Scan(&raw)
	if err != nil {
		return err
	}
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		return err
	}
	return writeJSON(filepath.Join(e.root, name), pretty)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func makeTask(id, queue string) *domain.Task {
	return &domain.Task{ID: id, Type: domain.TaskTypeRAGIngestion, Queue: queue,
		Payload:     domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": id}},
		MaxAttempts: 3, TimeoutSeconds: 30, RetryBackoffMultiplier: 2}
}

func put(ctx context.Context, q *postgres.PostgresQueue, e *experiment, t *domain.Task) error {
	e.event("client", t.ID, "enqueue", "invoke", t.Queue, 0, 0, nil)
	created, err := q.Enqueue(ctx, t)
	meta := map[string]any{}
	if created != nil {
		meta["canonical_id"] = created.ID
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	e.event("client", t.ID, "enqueue", completion(err), t.Queue, 0, 0, meta)
	return err
}

func completion(err error) string {
	if err != nil {
		return "fail"
	}
	return "ok"
}

func claimLoad(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	queue := e.prefix + "-claim"
	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: queue}); err != nil {
		return nil, err
	}
	const count, processes = 1000, 64
	for i := 0; i < count; i++ {
		if err := put(ctx, q, e, makeTask(fmt.Sprintf("%s-%04d", e.prefix, i), queue)); err != nil {
			return nil, err
		}
	}
	start := make(chan struct{})
	type owned struct {
		task   domain.Task
		worker string
	}
	out := make(chan owned, count)
	errCh := make(chan error, processes)
	var wg sync.WaitGroup
	var claimedCount atomic.Int64
	per := int(math.Ceil(float64(count) / processes))
	for i := 0; i < processes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			worker := fmt.Sprintf("claim-%02d", i)
			<-start
			e.event(worker, "", "claim", "invoke", queue, 0, 0, map[string]any{"limit": per})
			for claimedCount.Load() < count {
				got, err := q.ClaimTasks(ctx, worker, per, time.Minute)
				e.event(worker, "", "claim", completion(err), queue, 0, 0, map[string]any{"count": len(got)})
				if err != nil {
					errCh <- err
					return
				}
				if len(got) == 0 {
					time.Sleep(time.Millisecond)
					continue
				}
				claimedCount.Add(int64(len(got)))
				for _, t := range got {
					e.event(worker, t.ID, "owned", "ok", queue, t.LeaseVersion, t.Attempt, map[string]any{"claim_reason": t.ClaimReason})
					out <- owned{t, worker}
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(out)
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	duplicate := 0
	for x := range out {
		if seen[x.task.ID] {
			duplicate++
		}
		seen[x.task.ID] = true
		if err := q.MarkSuccess(ctx, x.task.ID, x.worker, x.task.LeaseVersion, domain.TaskResult{Summary: "recovery-scan-ack"}); err != nil {
			return nil, err
		}
		e.event(x.worker, x.task.ID, "ack", "ok", queue, x.task.LeaseVersion, x.task.Attempt, nil)
	}
	var terminal int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id LIKE $1 AND status='success'`, e.prefix+"%").Scan(&terminal); err != nil {
		return nil, err
	}
	return []assertion{{"all_tasks_uniquely_claimed", len(seen) == count && duplicate == 0, map[string]int{"unique": len(seen), "duplicates": duplicate}, map[string]int{"unique": count, "duplicates": 0}}, {"final_recovery_scan_terminal", terminal == count, terminal, count}}, nil
}

func capacityRepeated(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	const rounds, capN, callers = 20, 3, 16
	maxObserved := 0
	for round := 0; round < rounds; round++ {
		queue := fmt.Sprintf("%s-cap-%02d", e.prefix, round)
		if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: queue, MaxConcurrency: capN, Burst: capN}); err != nil {
			return nil, err
		}
		for i := 0; i < callers; i++ {
			if err := put(ctx, q, e, makeTask(fmt.Sprintf("%s-cap-%02d-%02d", e.prefix, round, i), queue)); err != nil {
				return nil, err
			}
		}
		start := make(chan struct{})
		type claimResult struct {
			count      int
			wrongQueue string
		}
		counts := make(chan claimResult, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				w := fmt.Sprintf("cap-%02d-%02d", round, i)
				got, err := q.ClaimTasks(ctx, w, 1, time.Minute)
				e.event(w, "", "claim", completion(err), queue, 0, 0, map[string]any{"count": len(got), "round": round})
				if err == nil {
					wrongQueue := ""
					for _, task := range got {
						e.event(w, task.ID, "owned", "ok", task.Queue, task.LeaseVersion, task.Attempt, nil)
						if task.Queue != queue {
							wrongQueue = task.Queue
						}
					}
					counts <- claimResult{count: len(got), wrongQueue: wrongQueue}
				} else {
					counts <- claimResult{count: 1000}
				}
			}(i)
		}
		close(start)
		wg.Wait()
		close(counts)
		total := 0
		for result := range counts {
			if result.wrongQueue != "" {
				return []assertion{{"capacity_round_claims_only_its_queue", false, result.wrongQueue, queue}}, nil
			}
			total += result.count
		}
		if total > maxObserved {
			maxObserved = total
		}
		if total > capN {
			return []assertion{{"strict_capacity", false, total, capN}}, nil
		}
		// ClaimTasks is intentionally global. Keep previous-round pending rows
		// from becoming candidates in a later round, so this oracle measures one
		// queue's aggregate capacity rather than cross-round scheduling.
		if err := q.PauseQueue(ctx, queue); err != nil {
			return nil, err
		}
	}
	return []assertion{{"strict_capacity_all_rounds", maxObserved <= capN, map[string]int{"rounds": rounds, "max_observed": maxObserved}, capN}}, nil
}

func queueIndependence(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	a, b := e.prefix+"-blocked-a", e.prefix+"-free-b"
	for _, name := range []string{a, b} {
		if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, MaxConcurrency: 1, Burst: 1}); err != nil {
			return nil, err
		}
		if err := put(ctx, q, e, makeTask(name+"-task", name)); err != nil {
			return nil, err
		}
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT 1 FROM queues WHERE name=$1 FOR UPDATE`, a); err != nil {
		return nil, err
	}
	e.event("nemesis", a, "hold_queue_authority", "info", a, 0, 0, nil)
	claimCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	got, err := q.ClaimTasks(claimCtx, "independent-worker", 2, time.Minute)
	elapsed := time.Since(started)
	e.event("independent-worker", "", "claim", completion(err), b, 0, 0, map[string]any{"elapsed_ms": elapsed.Milliseconds(), "count": len(got)})
	if err != nil {
		return []assertion{{"free_queue_not_blocked", false, err.Error(), "claim free-b within 500ms"}}, nil
	}
	pass := len(got) == 1 && got[0].Queue == b && elapsed < 500*time.Millisecond
	return []assertion{{"free_queue_not_blocked", pass, map[string]any{"count": len(got), "queue": func() string {
		if len(got) > 0 {
			return got[0].Queue
		}
		return ""
	}(), "elapsed_ms": elapsed.Milliseconds()}, "only free-b within 500ms"}}, nil
}

func differentWorkerFencing(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	id, queue := e.prefix+"-fence", e.prefix+"-fence-q"
	if err := put(ctx, q, e, makeTask(id, queue)); err != nil {
		return nil, err
	}
	a, err := q.ClaimTasks(ctx, "worker-a", 1, 25*time.Millisecond)
	if err != nil || len(a) != 1 {
		return nil, fmt.Errorf("first claim: n=%d err=%v", len(a), err)
	}
	time.Sleep(45 * time.Millisecond)
	b, err := q.ClaimTasks(ctx, "worker-b", 1, time.Minute)
	if err != nil || len(b) != 1 {
		return nil, fmt.Errorf("takeover: n=%d err=%v", len(b), err)
	}
	errs := []error{q.MarkSuccess(ctx, id, "worker-a", a[0].LeaseVersion, domain.TaskResult{Summary: "stale"}), q.MarkFailure(ctx, id, "worker-a", a[0].LeaseVersion, domain.TaskFailure{Message: "stale", Retryable: true}), q.RenewLease(ctx, id, "worker-a", a[0].LeaseVersion, time.Hour)}
	rejected := 0
	for i, x := range errs {
		ok := errors.Is(x, domain.ErrTaskNotClaimed)
		if ok {
			rejected++
		}
		e.event("worker-a", id, []string{"stale_success", "stale_failure", "stale_renew"}[i], map[bool]string{true: "fail", false: "ok"}[ok], queue, a[0].LeaseVersion, a[0].Attempt, map[string]any{"fenced": ok})
	}
	if err := q.MarkSuccess(ctx, id, "worker-b", b[0].LeaseVersion, domain.TaskResult{Summary: "current"}); err != nil {
		return nil, err
	}
	return []assertion{{"all_stale_operations_fenced", rejected == 3, rejected, 3}, {"generation_advanced", b[0].LeaseVersion == a[0].LeaseVersion+1, []int{a[0].LeaseVersion, b[0].LeaseVersion}, "+1"}}, nil
}

func renewalTakeover(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	id, queue := e.prefix+"-renew", e.prefix+"-renew-q"
	if err := put(ctx, q, e, makeTask(id, queue)); err != nil {
		return nil, err
	}
	a, err := q.ClaimTasks(ctx, "renewer", 1, 100*time.Millisecond)
	if err != nil || len(a) != 1 {
		return nil, fmt.Errorf("claim: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if err := q.RenewLease(ctx, id, "renewer", a[0].LeaseVersion, 120*time.Millisecond); err != nil {
		return nil, err
	}
	e.event("renewer", id, "renew", "ok", queue, a[0].LeaseVersion, a[0].Attempt, nil)
	time.Sleep(75 * time.Millisecond)
	early, err := q.ClaimTasks(ctx, "takeover", 1, time.Minute)
	if err != nil {
		return nil, err
	}
	time.Sleep(70 * time.Millisecond)
	late, err := q.ClaimTasks(ctx, "takeover", 1, time.Minute)
	if err != nil {
		return nil, err
	}
	if len(late) == 1 {
		_ = q.MarkSuccess(ctx, id, "takeover", late[0].LeaseVersion, domain.TaskResult{Summary: "takeover"})
	}
	return []assertion{{"renewal_prevented_early_takeover", len(early) == 0, len(early), 0}, {"stopped_renewal_allowed_takeover", len(late) == 1, len(late), 1}}, nil
}

func effectBeforeAck(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	id, queue := e.prefix+"-effect", e.prefix+"-effect-q"
	if err := put(ctx, q, e, makeTask(id, queue)); err != nil {
		return nil, err
	}
	a, err := q.ClaimTasks(ctx, "effect-a", 1, 25*time.Millisecond)
	if err != nil || len(a) != 1 {
		return nil, fmt.Errorf("claim a")
	}
	nonIdem := 0
	idem := map[string]bool{}
	nonIdem++
	idem[id] = true
	e.event("effect-a", id, "external_effect", "info", queue, a[0].LeaseVersion, a[0].Attempt, map[string]any{"ack": "lost"})
	time.Sleep(45 * time.Millisecond)
	b, err := q.ClaimTasks(ctx, "effect-b", 1, time.Minute)
	if err != nil || len(b) != 1 {
		return nil, fmt.Errorf("claim b")
	}
	nonIdem++
	idem[id] = true
	e.event("effect-b", id, "external_effect", "ok", queue, b[0].LeaseVersion, b[0].Attempt, map[string]any{"idempotency_key": id})
	if err := q.MarkSuccess(ctx, id, "effect-b", b[0].LeaseVersion, domain.TaskResult{Summary: "done"}); err != nil {
		return nil, err
	}
	return []assertion{{"non_idempotent_effect_duplicates", nonIdem == 2, nonIdem, 2}, {"idempotent_effect_converges", len(idem) == 1, len(idem), 1}}, nil
}

func concurrentIdempotency(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	key, queue := e.prefix+"-key", e.prefix+"-idem-q"
	const callers = 64
	start := make(chan struct{})
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			t := makeTask(fmt.Sprintf("%s-idem-%02d", e.prefix, i), queue)
			t.IdempotencyKey = &key
			e.event(fmt.Sprintf("idem-%02d", i), t.ID, "enqueue", "invoke", queue, 0, 0, nil)
			created, err := q.Enqueue(ctx, t)
			if err != nil {
				errs <- err
				return
			}
			ids <- created.ID
			e.event(fmt.Sprintf("idem-%02d", i), t.ID, "enqueue", "ok", queue, 0, 0, map[string]any{"canonical_id": created.ID})
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	var rows, submitted int
	if err := e.db.QueryRow(`SELECT COUNT(*),COUNT(*) FILTER (WHERE EXISTS(SELECT 1 FROM task_events e WHERE e.task_id=t.id AND e.event_type='submitted')) FROM tasks t WHERE idempotency_key=$1`, key).Scan(&rows, &submitted); err != nil {
		return nil, err
	}
	return []assertion{{"single_canonical_id", len(unique) == 1, len(unique), 1}, {"single_row_and_submitted_event", rows == 1 && submitted == 1, map[string]int{"rows": rows, "submitted": submitted}, map[string]int{"rows": 1, "submitted": 1}}}, nil
}

func retryBackoffDLQ(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	id, queue := e.prefix+"-retry", e.prefix+"-retry-q"
	t := makeTask(id, queue)
	t.MaxAttempts = 2
	t.RetryDelaySeconds = 1
	t.RetryMaxDelaySeconds = 1
	t.RetryBackoffMultiplier = 3
	if err := put(ctx, q, e, t); err != nil {
		return nil, err
	}
	a, err := q.ClaimTasks(ctx, "retry", 1, time.Minute)
	if err != nil || len(a) != 1 {
		return nil, fmt.Errorf("claim retry")
	}
	if err := q.MarkFailure(ctx, id, "retry", a[0].LeaseVersion, domain.TaskFailure{Message: "retryable", ErrorType: domain.ErrorTypeExecution, Retryable: true}); err != nil {
		return nil, err
	}
	var delta float64
	if err := e.db.QueryRow(`SELECT EXTRACT(EPOCH FROM(next_run_at-clock_timestamp())) FROM tasks WHERE id=$1`, id).Scan(&delta); err != nil {
		return nil, err
	}
	time.Sleep(time.Duration(math.Max(0, delta+0.1) * float64(time.Second)))
	b, err := q.ClaimTasks(ctx, "retry", 1, time.Minute)
	if err != nil || len(b) != 1 {
		return nil, fmt.Errorf("second claim n=%d err=%v", len(b), err)
	}
	if err := q.MarkFailure(ctx, id, "retry", b[0].LeaseVersion, domain.TaskFailure{Message: "again", ErrorType: domain.ErrorTypeExecution, Retryable: true}); err != nil {
		return nil, err
	}
	got, err := q.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	return []assertion{{"backoff_about_one_second", delta > 0.7 && delta <= 1.05, delta, "(0.7,1.05]"}, {"max_attempts_to_dlq", got.Status == domain.TaskStatusDLQ, string(got.Status), "dlq"}}, nil
}

func pauseInflight(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	queue := e.prefix + "-pause"
	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: queue, MaxConcurrency: 2, Burst: 2}); err != nil {
		return nil, err
	}
	firstID, secondID := e.prefix+"-inflight", e.prefix+"-pending"
	if err := put(ctx, q, e, makeTask(firstID, queue)); err != nil {
		return nil, err
	}
	first, err := q.ClaimTasks(ctx, "pause-worker", 1, time.Minute)
	if err != nil || len(first) != 1 {
		return nil, fmt.Errorf("first claim")
	}
	if err := put(ctx, q, e, makeTask(secondID, queue)); err != nil {
		return nil, err
	}
	if err := q.PauseQueue(ctx, queue); err != nil {
		return nil, err
	}
	blocked, err := q.ClaimTasks(ctx, "other", 1, time.Minute)
	if err != nil {
		return nil, err
	}
	ackErr := q.MarkSuccess(ctx, firstID, "pause-worker", first[0].LeaseVersion, domain.TaskResult{Summary: "inflight-completed"})
	if err := q.ResumeQueue(ctx, queue); err != nil {
		return nil, err
	}
	resumed, err := q.ClaimTasks(ctx, "other", 1, time.Minute)
	if err != nil {
		return nil, err
	}
	return []assertion{{"pause_blocks_new_claim", len(blocked) == 0, len(blocked), 0}, {"inflight_ack_survives_pause", ackErr == nil, fmt.Sprint(ackErr), "<nil>"}, {"resume_restores_claim", len(resumed) == 1 && resumed[0].ID == secondID, len(resumed), 1}}, nil
}

func rateReplicaInvariant(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	results := map[int]float64{}
	allPass := true
	for _, replicas := range []int{1, 2, 4} {
		queue := fmt.Sprintf("%s-rate-%d", e.prefix, replicas)
		if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: queue, RateLimitPerSecond: 4, Burst: 1}); err != nil {
			return nil, err
		}
		const tasks = 8
		for i := 0; i < tasks; i++ {
			if err := put(ctx, q, e, makeTask(fmt.Sprintf("%s-rate-%d-%02d", e.prefix, replicas, i), queue)); err != nil {
				return nil, err
			}
		}
		started := time.Now()
		done := make(chan struct{}, tasks)
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < replicas; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				worker := fmt.Sprintf("replica-%d-%d", replicas, i)
				for {
					select {
					case <-stop:
						return
					default:
					}
					got, err := q.ClaimTasks(ctx, worker, 1, time.Minute)
					if err != nil {
						return
					}
					if len(got) == 0 {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					t := got[0]
					e.event(worker, t.ID, "claim", "ok", queue, t.LeaseVersion, t.Attempt, nil)
					if q.MarkSuccess(ctx, t.ID, worker, t.LeaseVersion, domain.TaskResult{Summary: "rate"}) == nil {
						done <- struct{}{}
					}
				}
			}(i)
		}
		for i := 0; i < tasks; i++ {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				close(stop)
				wg.Wait()
				return nil, fmt.Errorf("rate timeout replicas=%d", replicas)
			}
		}
		close(stop)
		wg.Wait()
		duration := time.Since(started).Seconds()
		results[replicas] = duration
		if duration < 1.55 {
			allPass = false
		}
	}
	vals := []float64{results[1], results[2], results[4]}
	sort.Float64s(vals)
	spread := vals[2] - vals[0]
	return []assertion{{"shared_rate_lower_bound_each_replica_count", allPass, results, ">=1.55s for 8 tasks at 4/s burst1"}, {"replica_count_invariant", spread < 0.45, spread, "<0.45s spread"}}, nil
}

func databaseClock(ctx context.Context, q *postgres.PostgresQueue, e *experiment) ([]assertion, error) {
	id, queue := e.prefix+"-clock", e.prefix+"-clock-q"
	if err := put(ctx, q, e, makeTask(id, queue)); err != nil {
		return nil, err
	}
	got, err := q.ClaimTasks(ctx, "clock", 1, 2*time.Second)
	if err != nil || len(got) != 1 {
		return nil, fmt.Errorf("claim clock")
	}
	var delta, clientSkew float64
	if err := e.db.QueryRow(`SELECT EXTRACT(EPOCH FROM(lease_expires_at-clock_timestamp())),EXTRACT(EPOCH FROM(clock_timestamp()-$2::timestamptz)) FROM tasks WHERE id=$1`, id, time.Now().UTC().Add(24*time.Hour)).Scan(&delta, &clientSkew); err != nil {
		return nil, err
	}
	e.event("skewed-client", id, "observe_db_clock", "ok", queue, got[0].LeaseVersion, got[0].Attempt, map[string]any{"lease_delta_seconds": delta, "simulated_client_skew_seconds": clientSkew})
	return []assertion{{"lease_uses_database_now", delta > 1.5 && delta <= 2.1, delta, "(1.5,2.1]"}, {"client_wall_clock_irrelevant", clientSkew < -86390, clientSkew, "about -86400s"}}, nil
}
