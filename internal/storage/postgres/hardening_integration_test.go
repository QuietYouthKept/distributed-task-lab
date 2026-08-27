package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestQueueMaxConcurrencyConcurrent(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: "strict", MaxConcurrency: 1, Burst: 1}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		task := newTask(fmt.Sprintf("strict-%02d", i), 3)
		task.Queue = "strict"
		enqueue(t, q, task)
	}
	const claimers = 12
	ready, start := sync.WaitGroup{}, make(chan struct{})
	ready.Add(claimers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := 0
	for i := 0; i < claimers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ready.Done()
			<-start
			got, err := q.ClaimTasks(ctx, fmt.Sprintf("worker-%d", i), 1, time.Minute)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			claimed += len(got)
			mu.Unlock()
		}(i)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	var running int
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE queue='strict' AND status='running'`).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if running > 1 || claimed > 1 {
		t.Fatalf("capacity violated: running=%d returned=%d", running, claimed)
	}
}

func TestExpiredRunningReclaimIgnoresCapacity(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: "reclaim", MaxConcurrency: 1, Burst: 1}); err != nil {
		t.Fatal(err)
	}
	task := newTask("expired-at-capacity", 3)
	task.Queue = "reclaim"
	enqueue(t, q, task)
	first, err := q.ClaimTasks(ctx, "worker-a", 1, 2*time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: n=%d err=%v", len(first), err)
	}
	pending := newTask("high-priority-pending", 3)
	pending.Queue = "reclaim"
	pending.Priority = 1000
	enqueue(t, q, pending)
	time.Sleep(15 * time.Millisecond)
	second, err := q.ClaimTasks(ctx, "worker-b", 1, time.Minute)
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim: n=%d err=%v", len(second), err)
	}
	if second[0].LeaseVersion != first[0].LeaseVersion+1 || second[0].Attempt != first[0].Attempt+1 {
		t.Fatalf("generation not advanced: first=%+v second=%+v", first[0], second[0])
	}
	var running int
	_ = q.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE queue='reclaim' AND status='running'`).Scan(&running)
	if running != 1 {
		t.Fatalf("running=%d, want 1", running)
	}
}

func TestRenewLeaseRejectsRealStaleSameWorkerGeneration(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	enqueue(t, q, newTask("stale-renew", 3))
	first, _ := q.ClaimTasks(ctx, "same-worker", 1, 2*time.Millisecond)
	time.Sleep(15 * time.Millisecond)
	second, err := q.ClaimTasks(ctx, "same-worker", 1, time.Minute)
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim: n=%d err=%v", len(second), err)
	}
	before := *second[0].LeaseExpiresAt
	err = q.RenewLease(ctx, second[0].ID, "same-worker", first[0].LeaseVersion, time.Hour)
	if !errors.Is(err, domain.ErrTaskNotClaimed) {
		t.Fatalf("stale renew error=%v", err)
	}
	got, _ := q.GetTask(ctx, second[0].ID)
	if !got.LeaseExpiresAt.Equal(before) {
		t.Fatalf("stale renew changed generation 2 lease: %v -> %v", before, *got.LeaseExpiresAt)
	}
}

func TestConcurrentEnqueueIdempotency(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	const callers = 32
	key := "shared-idempotency"
	start := make(chan struct{})
	ids := make(chan string, callers)
	errCh := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			task := newTask(fmt.Sprintf("idem-%02d", i), 3)
			task.IdempotencyKey = &key
			created, err := q.Enqueue(ctx, task)
			if err != nil {
				errCh <- err
				return
			}
			ids <- created.ID
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errCh)
	for err := range errCh {
		t.Errorf("enqueue: %v", err)
	}
	var canonical string
	for id := range ids {
		if canonical == "" {
			canonical = id
		}
		if id != canonical {
			t.Errorf("different IDs: %s vs %s", canonical, id)
		}
	}
	var tasks, events int
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE idempotency_key=$1`, key).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM task_events e JOIN tasks t ON t.id=e.task_id WHERE t.idempotency_key=$1 AND e.event_type='submitted'`, key).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || events != 1 {
		t.Fatalf("tasks=%d submitted_events=%d, want 1/1", tasks, events)
	}
}

func TestGlobalQueueRateLimitAcrossWorkersAndStableRefresh(t *testing.T) {
	q1 := setupTestDB(t)
	q2 := NewPostgresQueue(q1.db)
	ctx := context.Background()
	cfg := domain.QueueConfig{Name: "global-rate", RateLimitPerSecond: 5, Burst: 1}
	if err := q1.SetQueueConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		task := newTask(fmt.Sprintf("rate-%d", i), 1)
		task.Queue = cfg.Name
		enqueue(t, q1, task)
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i, q := range []*PostgresQueue{q1, q2} {
		wg.Add(1)
		go func(i int, q *PostgresQueue) {
			defer wg.Done()
			<-start
			got, err := q.ClaimTasks(ctx, fmt.Sprintf("rate-worker-%d", i), 1, time.Minute)
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			results <- len(got)
		}(i, q)
	}
	close(start)
	wg.Wait()
	close(results)
	total := 0
	for n := range results {
		total += n
	}
	if total != 1 {
		t.Fatalf("combined immediate claims=%d, want 1", total)
	}
	if err := q1.SetQueueConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	immediate, err := q2.ClaimTasks(ctx, "refresh-worker", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(immediate) != 0 {
		t.Fatal("same-config refresh reset the shared burst")
	}
	time.Sleep(250 * time.Millisecond)
	after, err := q2.ClaimTasks(ctx, "later-worker", 1, time.Minute)
	if err != nil || len(after) != 1 {
		t.Fatalf("refilled claim n=%d err=%v", len(after), err)
	}
}

func TestLeaseUsesDatabaseClockAuthority(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	enqueue(t, q, newTask("db-clock", 1))
	claimed, err := q.ClaimTasks(ctx, "clock-worker", 1, 2*time.Second)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: n=%d err=%v", len(claimed), err)
	}
	var delta float64
	if err := q.db.QueryRow(`SELECT EXTRACT(EPOCH FROM (lease_expires_at-clock_timestamp())) FROM tasks WHERE id='db-clock'`).Scan(&delta); err != nil {
		t.Fatal(err)
	}
	if delta < 1.5 || delta > 2.1 {
		t.Fatalf("database lease delta=%fs, want about 2s", delta)
	}
}

// TestClaimSkipsContendedQueueAuthority proves that queue governance is an
// independent failure/admission domain: a transaction holding queue A's
// authority row must not prevent ready work in queue B from being claimed.
func TestClaimSkipsContendedQueueAuthority(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	for _, name := range []string{"blocked-a", "free-b"} {
		if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, MaxConcurrency: 1, Burst: 1}); err != nil {
			t.Fatal(err)
		}
		task := newTask(name+"-task", 1)
		task.Queue = name
		enqueue(t, q, task)
	}

	blocker, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.Exec(`SELECT 1 FROM queues WHERE name='blocked-a' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}

	claimCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	claimed, err := q.ClaimTasks(claimCtx, "independent-worker", 2, time.Minute)
	if err != nil {
		t.Fatalf("free queue claim was blocked by unrelated queue authority: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Queue != "free-b" {
		t.Fatalf("claimed=%v, want only free-b", claimed)
	}
}

func TestEnqueueCreatesQueueAndForeignKeyRejectsOrphan(t *testing.T) {
	q := setupTestDB(t)
	task := newTask("auto-queue", 1)
	task.Queue = "created-transactionally"
	enqueue(t, q, task)
	var queues int
	if err := q.db.QueryRow(`SELECT COUNT(*) FROM queues WHERE name='created-transactionally'`).Scan(&queues); err != nil {
		t.Fatal(err)
	}
	if queues != 1 {
		t.Fatalf("queue rows=%d", queues)
	}
	if _, err := q.db.Exec(`UPDATE tasks SET queue='missing-authority' WHERE id='auto-queue'`); err == nil {
		t.Fatal("foreign key allowed orphan queue")
	}
}

func TestDatabaseStateConstraintsRejectInvalidWrites(t *testing.T) {
	q := setupTestDB(t)
	enqueue(t, q, newTask("constraint-task", 1))
	for name, query := range map[string]string{
		"status":  `UPDATE tasks SET status='corrupt' WHERE id='constraint-task'`,
		"attempt": `UPDATE tasks SET attempt=-1 WHERE id='constraint-task'`,
		"timeout": `UPDATE tasks SET timeout_seconds=0 WHERE id='constraint-task'`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := q.db.Exec(query); err == nil {
				t.Fatalf("invalid %s write accepted", name)
			}
		})
	}
}

func TestPauseLinearizesAgainstClaim(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	mustQueue := domain.QueueConfig{Name: "pause-race", Burst: 1}
	if err := q.SetQueueConfig(ctx, mustQueue); err != nil {
		t.Fatal(err)
	}
	task := newTask("pause-race-task", 1)
	task.Queue = mustQueue.Name
	enqueue(t, q, task)
	pauseTx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pauseTx.Exec(`UPDATE queues SET paused=true WHERE name='pause-race'`); err != nil {
		t.Fatal(err)
	}
	result := make(chan []domain.Task, 1)
	errs := make(chan error, 1)
	go func() { got, err := q.ClaimTasks(ctx, "pause-race-worker", 1, time.Minute); result <- got; errs <- err }()
	// Claim skips a queue authority currently held by the pause transaction;
	// it must neither block nor cross that linearization point.
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	got := <-result
	if err := pauseTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("claim crossed committed pause: %d", len(got))
	}
}
