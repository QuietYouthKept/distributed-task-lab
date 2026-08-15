package worker

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func TestPoolThrottleRateLimit(t *testing.T) {
	q := &fakeQueue{}
	p := NewPool(q, "worker-1", 1, time.Second, 5*time.Second, 1, blockingHandler{}, discardLogger())

	// A "slow" queue with 2 tasks/sec and burst 1.
	p.limitersMu.Lock()
	p.limiters = map[string]*rate.Limiter{
		"slow": rate.NewLimiter(rate.Limit(2), 1),
	}
	p.limitersMu.Unlock()

	// First dispatch consumes the burst token immediately.
	if err := p.throttle(context.Background(), "slow"); err != nil {
		t.Fatalf("throttle 1: %v", err)
	}

	// Second dispatch must wait ~0.5s for the next token.
	start := time.Now()
	if err := p.throttle(context.Background(), "slow"); err != nil {
		t.Fatalf("throttle 2: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 300*time.Millisecond {
		t.Errorf("second throttle took %v, want >= 300ms (rate 2/s)", elapsed)
	}
}

func TestPoolThrottleUnlimitedQueue(t *testing.T) {
	q := &fakeQueue{}
	p := NewPool(q, "worker-1", 1, time.Second, 5*time.Second, 1, blockingHandler{}, discardLogger())

	// An unlimited queue has no limiter -> throttle is a no-op.
	start := time.Now()
	if err := p.throttle(context.Background(), "fast"); err != nil {
		t.Fatalf("throttle: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("unlimited queue throttle should be instant, took %v", elapsed)
	}
}

func TestPoolRefreshLimiters(t *testing.T) {
	q := &fakeQueue{
		queues: []domain.QueueConfig{
			{Name: "slow", RateLimitPerSecond: 5, Burst: 2},
			{Name: "fast", RateLimitPerSecond: 0},
		},
	}
	p := NewPool(q, "worker-1", 1, time.Second, 5*time.Second, 1, blockingHandler{}, discardLogger())

	p.refreshLimiters()

	p.limitersMu.Lock()
	defer p.limitersMu.Unlock()
	if p.limiters["slow"] == nil {
		t.Error("expected a limiter for 'slow' queue")
	}
	if p.limiters["fast"] != nil {
		t.Error("did not expect a limiter for 'fast' queue")
	}
}
