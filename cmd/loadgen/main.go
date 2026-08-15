// Command loadgen runs an in-process load test against PostgreSQL: it submits N
// tasks, runs M worker pools, and reports throughput, claim-latency percentiles,
// and duplicate-claim detection.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
	"github.com/fantasticyi/distributed-task-lab/internal/worker"
	"github.com/fantasticyi/distributed-task-lab/internal/worker/handlers"
)

// measuringQueue wraps a queue.Queue to record claim latency and detect
// duplicate claims across all workers.
type measuringQueue struct {
	queue.Queue
	mu        sync.Mutex
	claimed   map[string]int
	latencies []time.Duration
}

// ClaimTasks records latency and claimed IDs for every non-empty claim.
func (m *measuringQueue) ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error) {
	start := time.Now()
	tasks, err := m.Queue.ClaimTasks(ctx, workerID, limit, leaseTimeout)
	if len(tasks) > 0 {
		m.mu.Lock()
		m.latencies = append(m.latencies, time.Since(start))
		for _, t := range tasks {
			m.claimed[t.ID]++
		}
		m.mu.Unlock()
	}
	return tasks, err
}

func main() {
	url := envStr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tasklab?sslmode=disable")
	numTasks := envInt("LOADGEN_TASKS", 500)
	numWorkers := envInt("LOADGEN_WORKERS", 4)
	concurrency := envInt("LOADGEN_CONCURRENCY", 4)

	db, err := postgres.NewDB(url)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer db.Close()
	if err := postgres.RunMigrations(db); err != nil {
		fatal("migrations: %v", err)
	}

	q := postgres.NewPostgresQueue(db)
	mq := &measuringQueue{Queue: q, claimed: map[string]int{}}
	// Quiet logger so the report is readable.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	// Clean slate (load test expects a dedicated database).
	if _, err := db.Exec("DELETE FROM tasks"); err != nil {
		fatal("clean tasks: %v", err)
	}

	// Submit tasks.
	submitStart := time.Now()
	for i := 0; i < numTasks; i++ {
		task := &domain.Task{
			ID:             fmt.Sprintf("loadgen-%05d", i),
			Type:           domain.TaskTypeRAGIngestion,
			Payload:        domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": fmt.Sprintf("doc-%d", i), "mock_duration_ms": 1}},
			MaxAttempts:    1,
			TimeoutSeconds: 30,
		}
		if _, err := q.Enqueue(ctx, task); err != nil {
			fatal("enqueue: %v", err)
		}
	}
	submitDur := time.Since(submitStart)

	// Start worker pools.
	handler := handlers.NewMockHandler()
	var pools []*worker.Pool
	for i := 0; i < numWorkers; i++ {
		p := worker.NewPool(mq, fmt.Sprintf("loadgen-worker-%d", i), concurrency, 50*time.Millisecond, 30*time.Second, 20, handler, logger)
		p.Start()
		pools = append(pools, p)
	}

	// Wait for all tasks to reach a terminal state.
	runStart := time.Now()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		stats, err := q.Stats(ctx)
		if err != nil {
			fatal("stats: %v", err)
		}
		remaining := stats[domain.TaskStatusPending] + stats[domain.TaskStatusRunning]
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			fatal("load test timed out with %d tasks remaining", remaining)
		}
		time.Sleep(200 * time.Millisecond)
	}
	runDur := time.Since(runStart)

	for _, p := range pools {
		p.Stop()
	}

	stats, _ := q.Stats(ctx)
	duplicates := 0
	for _, n := range mq.claimed {
		if n > 1 {
			duplicates++
		}
	}

	fmt.Println("")
	fmt.Println("================ loadgen report ================")
	fmt.Printf("tasks submitted   : %d\n", numTasks)
	fmt.Printf("workers           : %d (concurrency %d each)\n", numWorkers, concurrency)
	fmt.Printf("submit time       : %s (%d/s)\n", submitDur.Round(time.Millisecond), int(float64(numTasks)/submitDur.Seconds()))
	fmt.Printf("processing time   : %s\n", runDur.Round(time.Millisecond))
	fmt.Printf("throughput        : %d tasks/sec\n", int(float64(numTasks)/runDur.Seconds()))
	fmt.Printf("success           : %d\n", stats[domain.TaskStatusSuccess])
	fmt.Printf("dlq               : %d\n", stats[domain.TaskStatusDLQ])
	fmt.Printf("claim latency p50 : %s\n", percentile(mq.latencies, 0.50).Round(time.Millisecond))
	fmt.Printf("claim latency p95 : %s\n", percentile(mq.latencies, 0.95).Round(time.Millisecond))
	fmt.Printf("claim latency p99 : %s\n", percentile(mq.latencies, 0.99).Round(time.Millisecond))
	fmt.Printf("duplicate claims  : %d\n", duplicates)
	fmt.Println("================================================")
}

// percentile returns the p-th percentile of the claim latencies.
func percentile(ds []time.Duration, p float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(ds))
	copy(sorted, ds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func envStr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func envInt(k string, fallback int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "loadgen: "+format+"\n", args...)
	os.Exit(1)
}
