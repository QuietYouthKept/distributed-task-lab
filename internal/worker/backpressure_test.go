package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
)

type backlogQueue struct {
	queue.Queue
	mu      sync.Mutex
	next    int
	claimed int
}

func (q *backlogQueue) ClaimTasks(_ context.Context, _ string, limit int, _ time.Duration) ([]domain.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]domain.Task, limit)
	for i := range result {
		q.next++
		result[i] = domain.Task{ID: fmt.Sprintf("task-%d", q.next), Type: domain.TaskTypeRAGIngestion,
			Payload: domain.TaskPayloadEnvelope{Data: map[string]any{}}, TimeoutSeconds: 10,
			Attempt: 1, LeaseVersion: 1}
	}
	q.claimed += len(result)
	return result, nil
}
func (q *backlogQueue) Heartbeat(context.Context, string) error { return nil }
func (q *backlogQueue) RenewLease(context.Context, string, string, int, time.Duration) error {
	return nil
}
func (q *backlogQueue) MarkFailure(context.Context, string, string, int, domain.TaskFailure) error {
	return nil
}
func (q *backlogQueue) MarkSuccess(context.Context, string, string, int, domain.TaskResult) error {
	return nil
}

func TestPoolBoundsClaimedNotStarted(t *testing.T) {
	q := &backlogQueue{}
	p := NewPool(q, "worker", 2, 5*time.Millisecond, time.Second, 50, blockingHandler{}, discardLogger())
	p.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = p.StopContext(ctx)
	})
	time.Sleep(100 * time.Millisecond)
	q.mu.Lock()
	claimed := q.claimed
	q.mu.Unlock()
	// owned limit = concurrency + min(batch, concurrency) = 4.
	if claimed > 4 {
		t.Fatalf("claimed %d tasks, want bounded at 4", claimed)
	}
}
