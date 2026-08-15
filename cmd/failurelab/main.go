// Command failurelab demonstrates the lease + fencing-token crash-recovery story:
// a worker claims a task, crashes, its lease expires, another worker reclaims the
// task, and the crashed worker's late write-back is rejected by the fencing token.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5433/tasklab_test?sslmode=disable"
	}

	db, err := postgres.NewDB(url)
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer db.Close()
	if err := postgres.RunMigrations(db); err != nil {
		fatalf("migrations: %v", err)
	}

	q := postgres.NewPostgresQueue(db)
	ctx := context.Background()

	// 1. Submit a long-running task.
	task := &domain.Task{
		ID:                fmt.Sprintf("failure-lab-%d", time.Now().UnixNano()),
		Type:              domain.TaskTypeRAGIngestion,
		Payload:           domain.TaskPayloadEnvelope{Version: "1", Kind: "rag_ingestion", Data: map[string]any{"document_id": "doc-crash"}},
		MaxAttempts:       3,
		TimeoutSeconds:    60,
		RetryDelaySeconds: 1,
	}
	if _, err := q.Enqueue(ctx, task); err != nil {
		fatalf("enqueue: %v", err)
	}
	fmt.Printf("1. task submitted: %s\n", task.ID)

	// 2. worker-a claims it with a short 2s lease.
	claimed, err := q.ClaimTasks(ctx, "worker-a", 1, 2*time.Second)
	if err != nil || len(claimed) != 1 {
		fatalf("worker-a claim: err=%v n=%d", err, len(claimed))
	}
	fmt.Printf("2. worker-a claimed: lease_version=%d, attempt=%d\n", claimed[0].LeaseVersion, claimed[0].Attempt)

	// 3. worker-a crashes without renewing its lease.
	fmt.Println("3. worker-a crashed (no heartbeat) — lease expires in ~2s")
	time.Sleep(2500 * time.Millisecond)

	// 4. worker-b reclaims the orphaned task; the fencing token advances.
	reclaimed, err := q.ClaimTasks(ctx, "worker-b", 1, 30*time.Second)
	if err != nil || len(reclaimed) != 1 {
		fatalf("worker-b reclaim: err=%v n=%d", err, len(reclaimed))
	}
	fmt.Printf("4. worker-b reclaimed: lease_version=%d, attempt=%d\n", reclaimed[0].LeaseVersion, reclaimed[0].Attempt)

	// 5. The crashed worker-a wakes up and tries to write back with its stale token.
	staleErr := q.MarkSuccess(ctx, task.ID, "worker-a", claimed[0].LeaseVersion, domain.TaskResult{Summary: "stale result"})
	fmt.Printf("5. stale write-back from worker-a (lease_version=%d): REJECTED (%v)\n", claimed[0].LeaseVersion, staleErr)

	// 6. worker-b completes the task with its current token.
	if err := q.MarkSuccess(ctx, task.ID, "worker-b", reclaimed[0].LeaseVersion, domain.TaskResult{Summary: "completed by worker-b"}); err != nil {
		fatalf("worker-b complete: %v", err)
	}
	fmt.Println("6. worker-b completed the task")

	// 7. Print the audit timeline.
	fmt.Println("\n--- task_events timeline ---")
	events, err := q.GetTaskEvents(ctx, task.ID)
	if err != nil {
		fatalf("events: %v", err)
	}
	for _, e := range events {
		worker := ""
		if e.WorkerID != nil {
			worker = *e.WorkerID
		}
		fmt.Printf("  %s  %-12s  worker=%-8s  %s\n", e.CreatedAt, e.EventType, worker, e.Message)
	}

	got, _ := q.GetTask(ctx, task.ID)
	fmt.Printf("\nfinal status: %s\n", got.Status)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "failurelab: "+format+"\n", args...)
	os.Exit(1)
}
