package worker

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
)

// fakeQueue implements queue.Queue for pool tests. It embeds a nil queue.Queue
// so unused methods are present but would panic if called; the pool only calls
// ClaimTasks, MarkSuccess, MarkFailure, and Heartbeat.
type fakeQueue struct {
	queue.Queue

	mu            sync.Mutex
	claimCalls    int
	markFailures  int
	markSuccesses int
	lastErrorType string
	lastCtxLive   bool
	claimed       bool
}

func (q *fakeQueue) ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claimCalls++
	if q.claimed {
		return nil, nil
	}
	q.claimed = true
	return []domain.Task{{
		ID:             "task-1",
		Type:           domain.TaskTypeRAGIngestion,
		Payload:        domain.TaskPayloadEnvelope{Data: map[string]any{}},
		TimeoutSeconds: 1,
		Attempt:        1,
		LeaseVersion:   1,
	}}, nil
}

func (q *fakeQueue) MarkFailure(ctx context.Context, taskID, workerID string, leaseVersion int, failure domain.TaskFailure) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.markFailures++
	q.lastErrorType = failure.ErrorType
	q.lastCtxLive = ctx.Err() == nil
	return nil
}

func (q *fakeQueue) MarkSuccess(ctx context.Context, taskID, workerID string, leaseVersion int, result domain.TaskResult) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.markSuccesses++
	return nil
}

func (q *fakeQueue) Heartbeat(ctx context.Context, workerID string) error { return nil }

// blockingHandler blocks until its context is cancelled or times out.
type blockingHandler struct{}

func (blockingHandler) Handle(ctx context.Context, taskType string, payload map[string]any, attempt int) (*TaskExecResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestPoolTimeoutPersistsFailure proves that a task that times out still has its
// failure persisted: the write context must not be cancelled by the execution
// deadline (regression test for the timeout-context bug).
func TestPoolTimeoutPersistsFailure(t *testing.T) {
	q := &fakeQueue{}
	p := NewPool(q, "worker-1", 1, 10*time.Millisecond, 5*time.Second, 1, blockingHandler{}, discardLogger())

	p.Start()
	defer p.Stop()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		n := q.markFailures
		q.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.markFailures == 0 {
		t.Fatal("expected MarkFailure to be called after task timeout")
	}
	if q.lastErrorType != domain.ErrorTypeTimeout {
		t.Errorf("ErrorType = %q, want %q", q.lastErrorType, domain.ErrorTypeTimeout)
	}
	if !q.lastCtxLive {
		t.Error("MarkFailure was called with an already-cancelled context (timeout write bug)")
	}
}

// TestPoolGracefulShutdown ensures Start/Stop terminate without hanging or panicking.
func TestPoolGracefulShutdown(t *testing.T) {
	q := &fakeQueue{}
	p := NewPool(q, "worker-1", 2, 10*time.Millisecond, 5*time.Second, 1, blockingHandler{}, discardLogger())

	p.Start()

	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not return within 3s")
	}
}
