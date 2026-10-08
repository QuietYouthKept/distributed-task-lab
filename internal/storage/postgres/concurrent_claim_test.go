package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestConcurrentClaimNoDuplicates is the core correctness test: N workers claim
// from a shared pool of M tasks concurrently. FOR UPDATE SKIP LOCKED must ensure
// no task is claimed by more than one worker. Run with -race to catch data races.
func TestConcurrentClaimNoDuplicates(t *testing.T) {
	q := setupTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const numTasks = 100
	for i := 0; i < numTasks; i++ {
		enqueue(t, q, newTask(fmt.Sprintf("test-conc-%03d", i), 3))
	}

	const numWorkers = 8
	var (
		mu        sync.Mutex
		claimed   []string
		wg        sync.WaitGroup
		claimErrs = make(chan error, numWorkers)
	)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			for {
				mu.Lock()
				complete := len(claimed) >= numTasks
				mu.Unlock()
				if complete {
					return
				}

				tasks, err := q.ClaimTasks(ctx, workerID, 10, 30*time.Second)
				if err != nil {
					if ctx.Err() == nil {
						claimErrs <- fmt.Errorf("claim (%s): %w", workerID, err)
					}
					return
				}
				if len(tasks) == 0 {
					// SKIP LOCKED can legitimately return no rows while another
					// worker owns the queue authority. An empty batch is not proof
					// that the shared queue is exhausted.
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Millisecond):
						continue
					}
				}
				mu.Lock()
				for _, task := range tasks {
					claimed = append(claimed, task.ID)
				}
				mu.Unlock()
			}
		}(fmt.Sprintf("worker-%d", w))
	}
	wg.Wait()
	close(claimErrs)
	for err := range claimErrs {
		t.Error(err)
	}

	seen := map[string]int{}
	for _, id := range claimed {
		seen[id]++
	}
	if len(seen) != numTasks {
		t.Errorf("expected %d unique claims before bounded deadline, got %d (context error: %v)", numTasks, len(seen), ctx.Err())
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("task %s claimed %d times (duplicate claim!)", id, n)
		}
	}
}
