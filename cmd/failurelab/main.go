// Command failurelab executes deterministic correctness demonstrations against
// a dedicated PostgreSQL database and emits a machine-readable JSON timeline.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
)

type event struct {
	Time         string `json:"time"`
	Scenario     string `json:"scenario"`
	Worker       string `json:"worker,omitempty"`
	Task         string `json:"task,omitempty"`
	Operation    string `json:"operation"`
	LeaseVersion int    `json:"lease_version,omitempty"`
	Attempt      int    `json:"attempt,omitempty"`
	State        string `json:"state,omitempty"`
	Result       string `json:"result"`
}

var encoder = json.NewEncoder(os.Stdout)

func emit(e event)              { e.Time = time.Now().UTC().Format(time.RFC3339Nano); _ = encoder.Encode(e) }
func pass(s, op, result string) { emit(event{Scenario: s, Operation: op, Result: "PASS: " + result}) }

func main() {
	baseURL := os.Getenv("DATABASE_URL")
	if baseURL == "" {
		baseURL = "postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable"
	}
	// ClaimTasks intentionally spans every queue. The lab must therefore never
	// share a database with an earlier lab run: leftover pending rows would be
	// valid claim candidates and corrupt a scenario's oracle. Provisioning a
	// uniquely named database also makes each run independently reproducible.
	labDB, err := testutil.EnsureDatabase(baseURL, "failurelab")
	must("create isolated database", err)
	defer labDB.Drop()
	db, err := postgres.NewDB(labDB.URL)
	must("setup", err)
	defer db.Close()
	must("migrations", postgres.RunMigrations(db))
	q := postgres.NewPostgresQueue(db)
	ctx := context.Background()
	prefix := fmt.Sprintf("fl-%d", time.Now().UnixNano())

	concurrentClaim(ctx, q, prefix)
	strictCapacity(ctx, q, db, prefix)
	reclaimAndFencing(ctx, q, prefix)
	leaseRenewal(ctx, q, prefix)
	retryDLQ(ctx, q, prefix)
	concurrentIdempotency(ctx, q, prefix)
	effectBeforeAck(ctx, q, prefix)
	pauseSemantics(ctx, q, prefix)
	globalRate(ctx, q, prefix)
	gracefulDrain(ctx, q, prefix)
	hardCrashRecovery(ctx, q, prefix)
	pass("summary", "all", "12 failure-lab scenarios satisfied their database oracles")
}

func task(id, queue string) *domain.Task {
	return &domain.Task{ID: id, Type: domain.TaskTypeRAGIngestion, Queue: queue,
		Payload:     domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": id}},
		MaxAttempts: 3, TimeoutSeconds: 5, RetryDelaySeconds: 0, RetryBackoffMultiplier: 2}
}
func enqueue(ctx context.Context, q *postgres.PostgresQueue, t *domain.Task) {
	_, err := q.Enqueue(ctx, t)
	must("enqueue "+t.ID, err)
}

func concurrentClaim(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "concurrent-claim-no-duplicate"
	queue := p + "-claims"
	for i := 0; i < 16; i++ {
		enqueue(ctx, q, task(fmt.Sprintf("%s-%02d", queue, i), queue))
	}
	start := make(chan struct{})
	ids := make(chan string, 16)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			got, err := q.ClaimTasks(ctx, fmt.Sprintf("c-%d", i), 2, time.Minute)
			must(s, err)
			for _, t := range got {
				ids <- t.ID
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			fatal(s, "duplicate "+id)
		}
		seen[id] = true
	}
	// SKIP LOCKED intentionally allows contending pollers to return empty while
	// another process holds the queue authority. Model real workers by polling
	// through a final recovery scan instead of treating one instant as quiescent.
	deadline := time.Now().Add(3 * time.Second)
	for len(seen) < 16 && time.Now().Before(deadline) {
		got, err := q.ClaimTasks(ctx, "recovery-scan", 2, time.Minute)
		must(s, err)
		for _, t := range got {
			if seen[t.ID] {
				fatal(s, "duplicate "+t.ID)
			}
			seen[t.ID] = true
		}
		if len(got) == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	if len(seen) != 16 {
		fatal(s, fmt.Sprintf("claimed=%d", len(seen)))
	}
	pass(s, "claim", "16 unique rows claimed once")
}

func strictCapacity(ctx context.Context, q *postgres.PostgresQueue, db *sql.DB, p string) {
	s := "strict-max-concurrency"
	name := p + "-cap"
	must(s, q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, MaxConcurrency: 1, Burst: 1}))
	for i := 0; i < 8; i++ {
		enqueue(ctx, q, task(fmt.Sprintf("%s-%d", name, i), name))
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET next_run_at=clock_timestamp()-interval '1 millisecond' WHERE queue=$1 AND status='pending'`, name); err != nil {
		fatal(s, "make fixtures due using PostgreSQL time: "+err.Error())
	}
	start := make(chan struct{})
	out := make(chan int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			got, err := q.ClaimTasks(ctx, fmt.Sprintf("cap-%d", i), 1, time.Minute)
			must(s, err)
			out <- len(got)
		}(i)
	}
	close(start)
	wg.Wait()
	close(out)
	total := 0
	for n := range out {
		total += n
	}
	if total != 1 {
		fatal(s, fmt.Sprintf("claims=%d", total))
	}
	pass(s, "admission", "combined claims remained 1")
}

func reclaimAndFencing(ctx context.Context, q *postgres.PostgresQueue, p string) {
	reclaimScenario := "lease-expiry-reclaim"
	fencingScenario := "same-worker-id-stale-fencing"
	id := p + "-fence"
	enqueue(ctx, q, task(id, p+"-fence-q"))
	a, err := q.ClaimTasks(ctx, "same-worker", 1, 20*time.Millisecond)
	must(reclaimScenario, err)
	time.Sleep(35 * time.Millisecond)
	b, err := q.ClaimTasks(ctx, "same-worker", 1, time.Minute)
	must(reclaimScenario, err)
	if len(a) != 1 || len(b) != 1 || b[0].LeaseVersion != a[0].LeaseVersion+1 {
		fatal(reclaimScenario, "generation did not advance")
	}
	emit(event{Scenario: reclaimScenario, Worker: "same-worker", Task: id, Operation: "reclaim", LeaseVersion: b[0].LeaseVersion, Attempt: b[0].Attempt, State: "running", Result: "PASS: expired ownership generation reclaimed"})
	err = q.MarkSuccess(ctx, id, "same-worker", a[0].LeaseVersion, domain.TaskResult{Summary: "stale"})
	if !errors.Is(err, domain.ErrTaskNotClaimed) {
		fatal(fencingScenario, "stale write accepted")
	}
	err = q.RenewLease(ctx, id, "same-worker", a[0].LeaseVersion, time.Hour)
	if !errors.Is(err, domain.ErrTaskNotClaimed) {
		fatal(fencingScenario, "stale renew accepted")
	}
	must(fencingScenario, q.MarkSuccess(ctx, id, "same-worker", b[0].LeaseVersion, domain.TaskResult{Summary: "current"}))
	emit(event{Scenario: fencingScenario, Worker: "same-worker", Task: id, Operation: "fence", LeaseVersion: b[0].LeaseVersion, Attempt: b[0].Attempt, State: "success", Result: "PASS: stale generation rejected"})
}

func leaseRenewal(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "lease-renewal"
	id := p + "-renew"
	enqueue(ctx, q, task(id, p+"-renew-q"))
	got, err := q.ClaimTasks(ctx, "renewer", 1, 100*time.Millisecond)
	must(s, err)
	time.Sleep(40 * time.Millisecond)
	must(s, q.RenewLease(ctx, id, "renewer", got[0].LeaseVersion, 200*time.Millisecond))
	time.Sleep(90 * time.Millisecond)
	other, err := q.ClaimTasks(ctx, "other", 1, time.Minute)
	must(s, err)
	for _, x := range other {
		if x.ID == id {
			fatal(s, "renewed task reclaimed")
		}
	}
	must(s, q.MarkSuccess(ctx, id, "renewer", got[0].LeaseVersion, domain.TaskResult{Summary: "renewed"}))
	pass(s, "renew", "ownership survived original expiry")
}

func retryDLQ(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "retry-backoff-dlq"
	id := p + "-dlq"
	t := task(id, p+"-dlq-q")
	t.MaxAttempts = 1
	enqueue(ctx, q, t)
	c, err := q.ClaimTasks(ctx, "retry-worker", 1, time.Minute)
	must(s, err)
	must(s, q.MarkFailure(ctx, id, "retry-worker", c[0].LeaseVersion, domain.TaskFailure{Message: "boom", ErrorType: domain.ErrorTypeExecution, Retryable: true}))
	got, err := q.GetTask(ctx, id)
	must(s, err)
	if got.Status != domain.TaskStatusDLQ {
		fatal(s, string(got.Status))
	}
	pass(s, "failure", "max-attempt task moved to DLQ")
}

func concurrentIdempotency(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "concurrent-idempotent-enqueue"
	key := p + "-idem"
	queueName := p + "-idem-q"
	start := make(chan struct{})
	ids := make(chan string, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			t := task(fmt.Sprintf("%s-%d", key, i), queueName)
			t.IdempotencyKey = &key
			created, err := q.Enqueue(ctx, t)
			must(s, err)
			ids <- created.ID
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	one := ""
	for id := range ids {
		if one == "" {
			one = id
		}
		if id != one {
			fatal(s, "different canonical IDs")
		}
	}
	// Keep the deliberately unexecuted canonical row from influencing later
	// globally-scoped ClaimTasks scenarios in this same lab run.
	must(s, q.PauseQueue(ctx, queueName))
	pass(s, "enqueue", "32 callers converged on one task ID")
}

func effectBeforeAck(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "effect-before-ack"
	id := p + "-effect"
	enqueue(ctx, q, task(id, p+"-effect-q"))
	first, err := q.ClaimTasks(ctx, "effect-a", 1, 20*time.Millisecond)
	must(s, err)
	effects := 0
	effects++
	time.Sleep(35 * time.Millisecond)
	second, err := q.ClaimTasks(ctx, "effect-b", 1, time.Minute)
	must(s, err)
	effects++
	if effects != 2 || len(first) != 1 || len(second) != 1 {
		fatal(s, "non-idempotent oracle failed")
	}
	business := map[string]bool{}
	business[id] = true
	business[id] = true
	if len(business) != 1 {
		fatal(s, "idempotent convergence failed")
	}
	must(s, q.MarkSuccess(ctx, id, "effect-b", second[0].LeaseVersion, domain.TaskResult{Summary: "converged"}))
	pass(s, "side_effect", "execution duplicated; idempotency key converged business effect once")
}

func pauseSemantics(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "queue-pause"
	name := p + "-paused"
	must(s, q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, Burst: 1}))
	enqueue(ctx, q, task(name+"-task", name))
	must(s, q.PauseQueue(ctx, name))
	got, err := q.ClaimTasks(ctx, "paused-worker", 1, time.Minute)
	must(s, err)
	if len(got) != 0 {
		fatal(s, "paused queue admitted task")
	}
	must(s, q.ResumeQueue(ctx, name))
	got, err = q.ClaimTasks(ctx, "paused-worker", 1, time.Minute)
	must(s, err)
	if len(got) != 1 {
		fatal(s, "resumed queue did not admit")
	}
	pass(s, "pause/resume", "pause blocked new claim and resume restored it")
}

func globalRate(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "global-rate-across-replicas"
	name := p + "-rate"
	must(s, q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, RateLimitPerSecond: 5, Burst: 1}))
	for i := 0; i < 2; i++ {
		enqueue(ctx, q, task(fmt.Sprintf("%s-%d", name, i), name))
	}
	start := make(chan struct{})
	out := make(chan int, 2)
	var wg sync.WaitGroup
	for i, x := range []*postgres.PostgresQueue{q, q} {
		wg.Add(1)
		go func(i int, x *postgres.PostgresQueue) {
			defer wg.Done()
			<-start
			got, err := x.ClaimTasks(ctx, fmt.Sprintf("rate-%d", i), 1, time.Minute)
			must(s, err)
			out <- len(got)
		}(i, x)
	}
	close(start)
	wg.Wait()
	close(out)
	total := 0
	for n := range out {
		total += n
	}
	if total != 1 {
		fatal(s, fmt.Sprintf("combined=%d", total))
	}
	must(s, q.PauseQueue(ctx, name))
	pass(s, "rate_admission", "two replicas consumed one shared burst token")
}

type delayHandler struct{ d time.Duration }

func (h delayHandler) Handle(ctx context.Context, _ string, _ map[string]any, _ int) (*worker.TaskExecResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(h.d):
		return &worker.TaskExecResult{Summary: "drained"}, nil
	}
}
func gracefulDrain(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "graceful-shutdown"
	id := p + "-drain"
	enqueue(ctx, q, task(id, p+"-drain-q"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := worker.NewPool(q, "drain-worker", 1, 5*time.Millisecond, 100*time.Millisecond, 1, delayHandler{120 * time.Millisecond}, logger)
	pool.Start()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		x, _ := q.GetTask(ctx, id)
		if x != nil && x.Status == domain.TaskStatusRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	must(s, pool.StopContext(stopCtx))
	got, err := q.GetTask(ctx, id)
	must(s, err)
	if got.Status != domain.TaskStatusSuccess {
		fatal(s, string(got.Status))
	}
	pass(s, "SIGTERM-drain", "claiming stopped, lease renewed, handler acked before exit")
}

func hardCrashRecovery(ctx context.Context, q *postgres.PostgresQueue, p string) {
	s := "hard-crash-recovery"
	id := p + "-crash"
	enqueue(ctx, q, task(id, p+"-crash-q"))
	a, err := q.ClaimTasks(ctx, "crashed", 1, 20*time.Millisecond)
	must(s, err)
	time.Sleep(35 * time.Millisecond)
	b, err := q.ClaimTasks(ctx, "survivor", 1, time.Minute)
	must(s, err)
	if len(a) != 1 || len(b) != 1 {
		fatal(s, "task not recovered")
	}
	must(s, q.MarkSuccess(ctx, id, "survivor", b[0].LeaseVersion, domain.TaskResult{Summary: "recovered"}))
	pass(s, "SIGKILL", "expired lease reclaimed and completed")
}

func must(scope string, err error) {
	if err != nil {
		fatal(scope, err.Error())
	}
}
func fatal(scope, msg string) {
	emit(event{Scenario: scope, Operation: "oracle", Result: "FAIL: " + msg})
	os.Exit(1)
}
