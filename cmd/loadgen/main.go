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
	"strings"
	"sync"
	"sync/atomic"
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
	profile := envStr("LOADGEN_PROFILE", "constant")
	producers := envInt("LOADGEN_PRODUCERS", 4)
	numWorkers := envInt("LOADGEN_WORKERS", 4)
	concurrency := envInt("LOADGEN_CONCURRENCY", 4)
	queueCount := envInt("LOADGEN_QUEUES", 1)
	payloadSize := envInt("LOADGEN_PAYLOAD_SIZE", 128)
	ratePerSecond := envInt("LOADGEN_RATE", 0)
	duration := envDuration("LOADGEN_DURATION", 0)
	seed := envInt("LOADGEN_SEED", 20260821)
	outputRoot := envStr("LOADGEN_OUTPUT_DIR", "benchmarks")
	runID := envStr("LOADGEN_RUN_ID", time.Now().UTC().Format("20060102T150405Z"))
	producers = maxInt(1, producers)
	queueCount = maxInt(1, queueCount)
	payloadSize = maxInt(0, payloadSize)
	if duration > 0 && ratePerSecond > 0 {
		numTasks = int(duration.Seconds()) * ratePerSecond
	}

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

	for i := 0; i < queueCount; i++ {
		if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: fmt.Sprintf("loadgen-q-%d", i), Burst: 1}); err != nil {
			fatal("queue config: %v", err)
		}
	}

	// Start workers before producers for steady profiles. Backlog-drain and spike
	// intentionally build the entire backlog first.
	handler := handlers.NewMockHandler()
	var pools []*worker.Pool
	startPools := func() {
		if len(pools) > 0 {
			return
		}
		for i := 0; i < numWorkers; i++ {
			p := worker.NewPool(mq, fmt.Sprintf("loadgen-worker-%d", i), concurrency, 50*time.Millisecond, 30*time.Second, 20, handler, logger)
			p.Start()
			pools = append(pools, p)
		}
	}
	if profile != "backlog-drain" && profile != "spike" {
		startPools()
	}

	// Submit tasks with bounded producer concurrency and deterministic workload.
	submitStart := time.Now()
	indexes := make(chan int)
	var submitWG sync.WaitGroup
	var submitErrors atomic.Int64
	for p := 0; p < producers; p++ {
		submitWG.Add(1)
		go func() {
			defer submitWG.Done()
			for i := range indexes {
				queueName := fmt.Sprintf("loadgen-q-%d", (i+seed)%queueCount)
				t := &domain.Task{ID: fmt.Sprintf("loadgen-%d-%06d", seed, i), Type: domain.TaskTypeRAGIngestion, Queue: queueName,
					Payload:     domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": fmt.Sprintf("doc-%d", i), "padding": strings.Repeat("x", payloadSize), "mock_duration_ms": 1}},
					MaxAttempts: 1, TimeoutSeconds: 30, RetryBackoffMultiplier: 2}
				if _, err := q.Enqueue(ctx, t); err != nil {
					submitErrors.Add(1)
				}
			}
		}()
	}
	for i := 0; i < numTasks; i++ {
		indexes <- i
		if ratePerSecond > 0 && profile != "spike" && profile != "backlog-drain" {
			interval := time.Second / time.Duration(ratePerSecond)
			if profile == "ramp" {
				interval = interval * time.Duration(numTasks-i) / time.Duration(maxInt(1, numTasks))
			}
			time.Sleep(interval)
		}
	}
	close(indexes)
	submitWG.Wait()
	submitDur := time.Since(submitStart)

	startPools()

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
	oldest, _ := q.OldestPendingAge(ctx)
	var retries, reclaims int64
	_ = db.QueryRow(`SELECT COUNT(*) FROM task_events WHERE event_type='retried'`).Scan(&retries)
	_ = db.QueryRow(`SELECT COUNT(*) FROM task_events WHERE event_type='claimed' AND metadata->>'claim_reason'='expired_lease'`).Scan(&reclaims)
	rawSamples := make([]loadgenRawSample, 0, len(mq.latencies)+2*numTasks)
	for i, latency := range mq.latencies {
		rawSamples = append(rawSamples, loadgenRawSample{Metric: "claim_latency", TaskID: strconv.Itoa(i), ValueMS: float64(latency) / float64(time.Millisecond)})
	}
	queueWaits, endToEnds := []time.Duration{}, []time.Duration{}
	rows, err := db.Query(`SELECT id,
		EXTRACT(EPOCH FROM (started_at - created_at)) * 1000,
		EXTRACT(EPOCH FROM (finished_at - created_at)) * 1000
		FROM tasks WHERE status = 'success' ORDER BY id`)
	if err != nil {
		fatal("read completion latency: %v", err)
	}
	for rows.Next() {
		var id string
		var queueWaitMS, endToEndMS float64
		if err := rows.Scan(&id, &queueWaitMS, &endToEndMS); err != nil {
			rows.Close()
			fatal("scan completion latency: %v", err)
		}
		queueWaits, endToEnds = append(queueWaits, time.Duration(queueWaitMS*float64(time.Millisecond))), append(endToEnds, time.Duration(endToEndMS*float64(time.Millisecond)))
		rawSamples = append(rawSamples, loadgenRawSample{Metric: "queue_wait", TaskID: id, ValueMS: queueWaitMS}, loadgenRawSample{Metric: "end_to_end", TaskID: id, ValueMS: endToEndMS})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fatal("completion latency rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		fatal("close completion latency rows: %v", err)
	}
	summary := loadgenArtifactSummary{
		RunID: runID, Profile: profile, Tasks: numTasks, Producers: producers, Workers: numWorkers, Concurrency: concurrency, Queues: queueCount, PayloadBytes: payloadSize,
		SubmitSeconds: submitDur.Seconds(), ProcessingSeconds: runDur.Seconds(), Throughput: float64(numTasks) / runDur.Seconds(),
		Success: stats[domain.TaskStatusSuccess], DLQ: stats[domain.TaskStatusDLQ], SubmissionErrors: submitErrors.Load(), Retries: retries, Reclaims: reclaims, DuplicateClaims: int64(duplicates),
		ClaimP50MS: float64(percentile(mq.latencies, 0.50)) / float64(time.Millisecond), ClaimP95MS: float64(percentile(mq.latencies, 0.95)) / float64(time.Millisecond), ClaimP99MS: float64(percentile(mq.latencies, 0.99)) / float64(time.Millisecond),
		QueueWaitP50MS: float64(percentile(queueWaits, 0.50)) / float64(time.Millisecond), QueueWaitP95MS: float64(percentile(queueWaits, 0.95)) / float64(time.Millisecond), QueueWaitP99MS: float64(percentile(queueWaits, 0.99)) / float64(time.Millisecond),
		EndToEndP50MS: float64(percentile(endToEnds, 0.50)) / float64(time.Millisecond), EndToEndP95MS: float64(percentile(endToEnds, 0.95)) / float64(time.Millisecond), EndToEndP99MS: float64(percentile(endToEnds, 0.99)) / float64(time.Millisecond),
	}
	artifactDir, artifactErr := writeLoadgenArtifacts(outputRoot, runID, db, summary, rawSamples)
	if artifactErr != nil {
		fatal("write evidence bundle: %v", artifactErr)
	}

	fmt.Println("")
	fmt.Println("================ loadgen report ================")
	fmt.Printf("tasks submitted   : %d\n", numTasks)
	fmt.Printf("profile/seed      : %s / %d\n", profile, seed)
	fmt.Printf("producers/queues  : %d / %d\n", producers, queueCount)
	fmt.Printf("payload bytes     : %d\n", payloadSize)
	fmt.Printf("workers           : %d (concurrency %d each)\n", numWorkers, concurrency)
	fmt.Printf("submit time       : %s (%d/s)\n", submitDur.Round(time.Millisecond), int(float64(numTasks)/submitDur.Seconds()))
	fmt.Printf("processing time   : %s\n", runDur.Round(time.Millisecond))
	fmt.Printf("throughput        : %d tasks/sec\n", int(float64(numTasks)/runDur.Seconds()))
	fmt.Printf("success           : %d\n", stats[domain.TaskStatusSuccess])
	fmt.Printf("dlq               : %d\n", stats[domain.TaskStatusDLQ])
	fmt.Printf("submission errors : %d (%.3f%%)\n", submitErrors.Load(), 100*float64(submitErrors.Load())/float64(maxInt(1, numTasks)))
	fmt.Printf("oldest queue age  : %s\n", oldest.Round(time.Millisecond))
	fmt.Printf("retries/reclaims  : %d / %d\n", retries, reclaims)
	fmt.Printf("claim latency p50 : %s\n", percentile(mq.latencies, 0.50).Round(time.Millisecond))
	fmt.Printf("claim latency p95 : %s\n", percentile(mq.latencies, 0.95).Round(time.Millisecond))
	fmt.Printf("claim latency p99 : %s\n", percentile(mq.latencies, 0.99).Round(time.Millisecond))
	fmt.Printf("queue wait p95    : %s\n", percentile(queueWaits, 0.95).Round(time.Millisecond))
	fmt.Printf("end-to-end p95   : %s\n", percentile(endToEnds, 0.95).Round(time.Millisecond))
	fmt.Printf("duplicate claims  : %d\n", duplicates)
	fmt.Printf("evidence bundle  : %s\n", artifactDir)
	fmt.Println("================================================")
	if submitErrors.Load() != 0 {
		fatal("invalid run: %d task submissions failed", submitErrors.Load())
	}
	if duplicates != 0 {
		fatal("correctness oracle failed: %d task IDs were claimed more than once", duplicates)
	}
	if stats[domain.TaskStatusSuccess] != int64(numTasks) {
		fatal("invalid run: success=%d, want %d", stats[domain.TaskStatusSuccess], numTasks)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func envDuration(k string, fallback time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
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
