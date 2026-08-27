package postgres

import (
	"context"
	"fmt"
	"sync/atomic"
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
func BenchmarkEnqueueSequential(b *testing.B) {
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

func BenchmarkEnqueueParallel(b *testing.B) {
	q := benchQueue(b)
	var n atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := n.Add(1)
			if _, err := q.Enqueue(context.Background(), newTask(fmt.Sprintf("bench-par-%d", i), 3)); err != nil {
				b.Errorf("enqueue: %v", err)
				return
			}
		}
	})
}

func BenchmarkEnqueueIdempotencyConflict(b *testing.B) {
	q := benchQueue(b)
	ctx := context.Background()
	key := "bench-conflict"
	base := newTask("bench-conflict-canonical", 3)
	base.IdempotencyKey = &key
	if _, err := q.Enqueue(ctx, base); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := newTask(fmt.Sprintf("bench-conflict-%d", i), 3)
		t.IdempotencyKey = &key
		if _, err := q.Enqueue(ctx, t); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClaimSameQueue(b *testing.B) {
	q := benchQueue(b)
	ctx := context.Background()
	name := "bench-same"
	_ = q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, Burst: 1})
	for i := 0; i < b.N; i++ {
		t := newTask(fmt.Sprintf("bench-same-%d", i), 1)
		t.Queue = name
		if _, err := q.Enqueue(ctx, t); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := q.ClaimTasks(ctx, "bench", 1, time.Minute)
		if err != nil || len(got) != 1 {
			b.Fatalf("claim n=%d err=%v", len(got), err)
		}
		if err := q.MarkSuccess(ctx, got[0].ID, "bench", got[0].LeaseVersion, domain.TaskResult{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClaimMultipleQueues(b *testing.B) {
	q := benchQueue(b)
	ctx := context.Background()
	for _, name := range []string{"bench-a", "bench-b", "bench-c", "bench-d"} {
		_ = q.SetQueueConfig(ctx, domain.QueueConfig{Name: name, Burst: 1})
	}
	for i := 0; i < b.N; i++ {
		t := newTask(fmt.Sprintf("bench-multi-%d", i), 1)
		t.Queue = fmt.Sprintf("bench-%c", 'a'+rune(i%4))
		if _, err := q.Enqueue(ctx, t); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := q.ClaimTasks(ctx, "bench", 1, time.Minute)
		if err != nil || len(got) != 1 {
			b.Fatalf("claim n=%d err=%v", len(got), err)
		}
		if err := q.MarkSuccess(ctx, got[0].ID, "bench", got[0].LeaseVersion, domain.TaskResult{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTaskLifecycle measures the full enqueue -> claim -> success cycle,
// i.e. the end-to-end cost of processing one task.
func BenchmarkTaskLifecycleRepository(b *testing.B) {
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
