package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/testutil"
)

func benchQueue(b *testing.B) *PostgresQueue {
	b.Helper()
	tdb, err := testutil.EnsureDatabase(testutil.DefaultBaseURL(), "tasklab_bench")
	if err != nil {
		b.Skipf("skipping benchmark: %v", err)
	}
	db, err := NewDB(tdb.URL)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		b.Fatalf("migrations: %v", err)
	}
	b.Cleanup(func() {
		db.Close()
		tdb.Drop()
	})
	return NewPostgresQueue(db)
}

// BenchmarkEnqueue measures single-task enqueue throughput (idempotent insert).
func BenchmarkEnqueue(b *testing.B) {
	q := benchQueue(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := newTask(fmt.Sprintf("bench-enq-%d", i), 3)
		if _, err := q.Enqueue(ctx, task); err != nil {
			b.Fatalf("enqueue: %v", err)
		}
	}
}

// BenchmarkTaskLifecycle measures the full enqueue -> claim -> success cycle,
// i.e. the end-to-end cost of processing one task.
func BenchmarkTaskLifecycle(b *testing.B) {
	q := benchQueue(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := newTask(fmt.Sprintf("bench-life-%d", i), 3)
		if _, err := q.Enqueue(ctx, task); err != nil {
			b.Fatalf("enqueue: %v", err)
		}
		claimed, err := q.ClaimTasks(ctx, "bench-worker", 1, 30*time.Second)
		if err != nil {
			b.Fatalf("claim: %v", err)
		}
		if len(claimed) != 1 {
			b.Fatal("expected 1 claim")
		}
		if err := q.MarkSuccess(ctx, claimed[0].ID, "bench-worker", claimed[0].LeaseVersion, domain.TaskResult{Summary: "ok"}); err != nil {
			b.Fatalf("mark success: %v", err)
		}
	}
}
