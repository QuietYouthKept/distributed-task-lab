package postgres

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

// TestConcurrentClaimNoDuplicates is the core correctness test: N workers claim
// from a shared pool of M tasks concurrently. FOR UPDATE SKIP LOCKED must ensure
// no task is claimed by more than one worker. Optional TASKLAB_CLAIM_* settings
// drive the separately gated 100-worker/10k-task acceptance profile.
func TestConcurrentClaimNoDuplicates(t *testing.T) {
	q := setupTestDB(t)
	numTasks := boundedTestInt(t, "TASKLAB_CLAIM_TASKS", 100, 1, 100000)
	numWorkers := boundedTestInt(t, "TASKLAB_CLAIM_WORKERS", 8, 1, 100)
	timeoutSeconds := boundedTestInt(t, "TASKLAB_CLAIM_TIMEOUT_SECONDS", 10, 1, 3600)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	for i := 0; i < numTasks; i++ {
		enqueue(t, q, newTask(fmt.Sprintf("test-conc-%06d", i), 3))
	}

	var (
		mu        sync.Mutex
		claimed   []string
		wg        sync.WaitGroup
		claimErrs = make(chan error, 2*numTasks+numWorkers)
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
				for _, task := range tasks {
					if task.Attempt != 1 || task.LeaseVersion != 1 || task.ClaimReason != "pending" {
						claimErrs <- fmt.Errorf("claim (%s): task %s attempt=%d lease_version=%d reason=%q; want 1/1/pending", workerID, task.ID, task.Attempt, task.LeaseVersion, task.ClaimReason)
					}
					if err := q.MarkSuccess(ctx, task.ID, workerID, task.LeaseVersion, domain.TaskResult{Summary: "concurrent claim accepted"}); err != nil {
						claimErrs <- fmt.Errorf("ack (%s, %s): %w", workerID, task.ID, err)
					}
				}
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
	var terminalSuccess, lifecycleEvents int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id LIKE 'test-conc-%' AND status='success'`).Scan(&terminalSuccess); err != nil {
		t.Fatalf("count terminal successful tasks: %v", err)
	}
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_events WHERE task_id LIKE 'test-conc-%' AND event_type IN ('claimed','succeeded')`).Scan(&lifecycleEvents); err != nil {
		t.Fatalf("count claim/success events: %v", err)
	}
	if terminalSuccess != numTasks || lifecycleEvents != 2*numTasks {
		t.Fatalf("persisted lifecycle mismatch: success=%d events=%d, want success=%d events=%d", terminalSuccess, lifecycleEvents, numTasks, 2*numTasks)
	}
}

func boundedTestInt(t *testing.T, name string, fallback, minValue, maxValue int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > maxValue {
		t.Fatalf("%s must be an integer in [%d,%d]", name, minValue, maxValue)
	}
	return value
}
