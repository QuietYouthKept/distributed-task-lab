package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestQueuePausePreventsClaim(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: "batch"}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	task := newTask("test-pause-001", 3)
	task.Queue = "batch"
	enqueue(t, q, task)

	if err := q.PauseQueue(ctx, "batch"); err != nil {
		t.Fatalf("PauseQueue: %v", err)
	}

	claimed, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 claims from paused queue, got %d", len(claimed))
	}

	if err := q.ResumeQueue(ctx, "batch"); err != nil {
		t.Fatalf("ResumeQueue: %v", err)
	}
	claimed, err = q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks after resume: %v", err)
	}
	if len(claimed) != 1 {
		t.Errorf("expected 1 claim after resume, got %d", len(claimed))
	}
}

func TestQueueMaxConcurrency(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: "qc", MaxConcurrency: 1}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	for i := 0; i < 3; i++ {
		task := newTask(fmt.Sprintf("test-qc-%d", i), 3)
		task.Queue = "qc"
		enqueue(t, q, task)
	}

	claimed, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if len(claimed) != 1 {
		t.Errorf("expected 1 claim (max_concurrency=1), got %d", len(claimed))
	}

	claimed2, err := q.ClaimTasks(ctx, "worker-1", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimTasks 2: %v", err)
	}
	if len(claimed2) != 0 {
		t.Errorf("expected 0 claims while at capacity, got %d", len(claimed2))
	}
}

func TestListQueues(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()

	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: "listme", MaxConcurrency: 4, RateLimitPerSecond: 2.5, AgingAfterSeconds: 30, AgingPriorityBoost: 50}); err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}

	queues, err := q.ListQueues(ctx)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	found := false
	for _, c := range queues {
		if c.Name == "listme" {
			found = true
			if c.MaxConcurrency != 4 {
				t.Errorf("MaxConcurrency = %d, want 4", c.MaxConcurrency)
			}
			if c.RateLimitPerSecond != 2.5 {
				t.Errorf("RateLimitPerSecond = %v, want 2.5", c.RateLimitPerSecond)
			}
			if c.AgingAfterSeconds != 30 || c.AgingPriorityBoost != 50 {
				t.Errorf("aging = %d/%d, want 30/50", c.AgingAfterSeconds, c.AgingPriorityBoost)
			}
		}
	}
	if !found {
		t.Error("queue 'listme' not found in ListQueues")
	}
}

func TestQueueAgingPromotesOldPendingTask(t *testing.T) {
	q := setupTestDB(t)
	ctx := context.Background()
	if err := q.SetQueueConfig(ctx, domain.QueueConfig{Name: "aged", AgingAfterSeconds: 1, AgingPriorityBoost: 20}); err != nil {
		t.Fatal(err)
	}
	old := newTask("aged-low", 1)
	old.Queue, old.Priority = "aged", 0
	enqueue(t, q, old)
	if _, err := q.db.ExecContext(ctx, `UPDATE tasks SET created_at = clock_timestamp() - interval '2 seconds' WHERE id = $1`, old.ID); err != nil {
		t.Fatal(err)
	}
	newer := newTask("aged-high", 1)
	newer.Queue, newer.Priority = "aged", 10
	enqueue(t, q, newer)
	claimed, err := q.ClaimTasks(ctx, "aging-worker", 1, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimTasks = %#v, %v", claimed, err)
	}
	if claimed[0].ID != old.ID {
		t.Fatalf("claimed %s, want aged low priority task", claimed[0].ID)
	}
	events, err := q.GetTaskEvents(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.EventType == domain.EventPriorityPromoted {
			found = true
		}
	}
	if !found {
		t.Fatal("missing priority promotion audit event")
	}
}
