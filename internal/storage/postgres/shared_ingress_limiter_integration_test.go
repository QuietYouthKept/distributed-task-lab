package postgres

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedIngressLimiterCoordinatesIndependentInstancesAndBoundsState(t *testing.T) {
	q := setupTestDB(t)
	key := []byte("wave4-shared-ingress-hash-key-32-bytes")
	one, err := NewSharedIngressLimiter(q.db, key, 0.001, 16)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewSharedIngressLimiter(q.db, key, 0.001, 16)
	if err != nil {
		t.Fatal(err)
	}
	const requests = 80
	var allowed atomic.Int32
	var denied atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		limiter := one
		if i%2 == 1 {
			limiter = two
		}
		wg.Add(1)
		go func(l *SharedIngressLimiter) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ok, retryAfter, err := l.Allow(ctx, "tenant-A/user-7")
			if err != nil {
				failures.Add(1)
				return
			}
			if ok {
				allowed.Add(1)
			} else {
				if retryAfter < 1 {
					failures.Add(1)
				}
				denied.Add(1)
			}
		}(limiter)
	}
	close(start)
	wg.Wait()
	if failures.Load() != 0 || allowed.Load() != 16 || denied.Load() != requests-16 {
		t.Fatalf("two replicas admitted/denied/errors = %d/%d/%d, want 16/%d/0", allowed.Load(), denied.Load(), failures.Load(), requests-16)
	}
	if ok, _, err := two.Allow(context.Background(), "tenant-B/user-7"); err != nil || !ok {
		t.Fatalf("independent principal admission = allowed %t, err %v", ok, err)
	}
	var slots, occupied, rawSubjectMatches int
	if err := q.db.QueryRow(`
		SELECT COUNT(*), COUNT(*) FILTER (WHERE principal_hash IS NOT NULL),
		       COUNT(*) FILTER (WHERE principal_hash = convert_to('tenant-A/user-7','UTF8'))
		FROM api_ingress_rate_limit_buckets
	`).Scan(&slots, &occupied, &rawSubjectMatches); err != nil {
		t.Fatal(err)
	}
	if slots != sharedIngressBucketCapacity || occupied != 2 || rawSubjectMatches != 0 {
		t.Fatalf("bounded state slots/occupied/raw-subject-match = %d/%d/%d", slots, occupied, rawSubjectMatches)
	}
}
