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
	queues        []domain.QueueConfig
	renewCalls    int
	renewErr      error
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

func (q *fakeQueue) RenewLease(ctx context.Context, taskID, workerID string, leaseVersion int, leaseTimeout time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.renewCalls++
	return q.renewErr
}

func (q *fakeQueue) ListQueues(ctx context.Context) ([]domain.QueueConfig, error) {
	return q.queues, nil
}

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

type delayedSuccessHandler struct{ delay time.Duration }

func (h delayedSuccessHandler) Handle(ctx context.Context, _ string, _ map[string]any, _ int) (*TaskExecResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(h.delay):
		return &TaskExecResult{Summary: "done"}, nil
	}
}

func TestPoolRenewsLeaseDuringLongHandler(t *testing.T) {
	q := &fakeQueue{}
	p := NewPool(q, "worker-1", 1, 5*time.Millisecond, 90*time.Millisecond, 1, delayedSuccessHandler{220 * time.Millisecond}, discardLogger())
	p.Start()
	t.Cleanup(p.Stop)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		done, renews := q.markSuccesses, q.renewCalls
		q.mu.Unlock()
		if done == 1 {
			if renews < 2 {
				t.Fatalf("renew calls = %d, want at least 2", renews)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("long handler did not complete")
}

func TestPoolOwnershipLossCancelsAndDoesNotAck(t *testing.T) {
	q := &fakeQueue{renewErr: domain.ErrTaskNotClaimed}
	p := NewPool(q, "worker-1", 1, 5*time.Millisecond, 60*time.Millisecond, 1, blockingHandler{}, discardLogger())
	p.Start()
	t.Cleanup(p.Stop)
	time.Sleep(150 * time.Millisecond)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.renewCalls == 0 {
		t.Fatal("expected lease renewal")
	}
	if q.markSuccesses != 0 || q.markFailures != 0 {
		t.Fatalf("stale execution acked: success=%d failure=%d", q.markSuccesses, q.markFailures)
	}
}

type cancellationRaceQueue struct {
	*fakeQueue
	renewEntered chan struct{}
	once         sync.Once
}

func (q *cancellationRaceQueue) RenewLease(ctx context.Context, taskID, workerID string, leaseVersion int, leaseTimeout time.Duration) error {
	q.mu.Lock()
	q.renewCalls++
	q.mu.Unlock()
	q.once.Do(func() { close(q.renewEntered) })
	<-ctx.Done()
	return ctx.Err()
}

type finishDuringRenewHandler struct{ renewEntered <-chan struct{} }

func (h finishDuringRenewHandler) Handle(ctx context.Context, _ string, _ map[string]any, _ int) (*TaskExecResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.renewEntered:
		return &TaskExecResult{Summary: "completed while renewal was in flight"}, nil
	}
}

// TestPoolLocalRenewCancellationDoesNotDiscardResult is the deterministic
// regression for the Docker-discovered race where handler completion cancels
// an in-flight renewal SQL call. That local cancellation is not lease loss.
func TestPoolLocalRenewCancellationDoesNotDiscardResult(t *testing.T) {
	entered := make(chan struct{})
	q := &cancellationRaceQueue{fakeQueue: &fakeQueue{}, renewEntered: entered}
	p := NewPool(q, "worker-1", 1, 5*time.Millisecond, 60*time.Millisecond, 1, finishDuringRenewHandler{entered}, discardLogger())
	p.Start()
	t.Cleanup(p.Stop)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		successes, failures := q.markSuccesses, q.markFailures
		q.mu.Unlock()
		if successes == 1 {
			if failures != 0 {
				t.Fatalf("unexpected failures=%d", failures)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("result was discarded when local renewal cancellation raced completion")
}
