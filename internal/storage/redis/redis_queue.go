package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/queue"
)

// Ensure RedisQueue implements queue.Queue.
var _ queue.Queue = (*RedisQueue)(nil)

// RedisQueue implements the Queue interface using Redis.
// Uses List for ready queue, ZSet for delayed retry, Hash for task metadata.
//
// Key structure:
//   queue:ready          — List of task IDs ready to be claimed
//   queue:delayed        — ZSet of task IDs with next_run_at as score
//   tasks:{task_id}      — Hash storing full task JSON
//   queue:dlq            — List of DLQ task IDs
type RedisQueue struct {
	// In a real implementation, this would hold a redis.Client.
	// For Phase 6 skeleton, we define the structure and key patterns.
	// client *redis.Client
}

// NewRedisQueue creates a new Redis-backed queue.
func NewRedisQueue(redisURL string) (*RedisQueue, error) {
	// In a real implementation: parse URL, connect, ping.
	// opt, _ := redis.ParseURL(redisURL)
	// client := redis.NewClient(opt)
	return &RedisQueue{}, nil
}

// Enqueue inserts a new task. In Redis, this means:
// 1. Store task JSON in tasks:{id} hash
// 2. Push task ID to queue:ready list
func (q *RedisQueue) Enqueue(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	now := time.Now().UTC()
	task.Status = domain.TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	task.Attempt = 0
	if task.NextRunAt.IsZero() {
		task.NextRunAt = now
	}

	// Redis pseudocode:
	// data, _ := json.Marshal(task)
	// pipe := q.client.Pipeline()
	// pipe.HSet(ctx, fmt.Sprintf("tasks:%s", task.ID), "data", data)
	// pipe.LPush(ctx, "queue:ready", task.ID)
	// _, err := pipe.Exec(ctx)
	return nil, fmt.Errorf("redis: %w", domain.ErrNotImplemented)
}

// GetTask retrieves a task by ID from its hash.
func (q *RedisQueue) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	// data, err := q.client.HGet(ctx, fmt.Sprintf("tasks:%s", taskID), "data").Bytes()
	// if err == redis.Nil { return nil, nil }
	// var task domain.Task; json.Unmarshal(data, &task); return &task, nil
	return nil, domain.ErrNotImplemented
}

// ListTasks would scan all task:* keys and filter — expensive in Redis,
// which is one reason PostgreSQL is a better fit for query-heavy operations.
func (q *RedisQueue) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	return nil, domain.ErrNotImplemented
}

// ClaimTasks atomically claims tasks using a Lua script:
// 1. Pop up to `limit` IDs from queue:ready
// 2. Also check queue:delayed for IDs whose score <= now
// 3. For each ID, update task status to running, set lease fields
// 4. Return claimed tasks
func (q *RedisQueue) ClaimTasks(ctx context.Context, workerID string, limit int, leaseTimeout time.Duration) ([]domain.Task, error) {
	return nil, domain.ErrNotImplemented
}

// MarkSuccess updates the task hash and removes from running set.
func (q *RedisQueue) MarkSuccess(ctx context.Context, taskID string, workerID string, leaseVersion int, result domain.TaskResult) error {
	return domain.ErrNotImplemented
}

// MarkFailure handles retry/DLQ using ZSet for delayed retry.
func (q *RedisQueue) MarkFailure(ctx context.Context, taskID string, workerID string, leaseVersion int, failure domain.TaskFailure) error {
	return domain.ErrNotImplemented
}

// ListDLQ reads from queue:dlq list.
func (q *RedisQueue) ListDLQ(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	return nil, domain.ErrNotImplemented
}

// Requeue moves a task from queue:dlq back to queue:ready.
func (q *RedisQueue) Requeue(ctx context.Context, taskID string, reason string) error {
	return domain.ErrNotImplemented
}

// Cancel updates task status in its hash.
func (q *RedisQueue) Cancel(ctx context.Context, taskID string, reason string) error {
	return domain.ErrNotImplemented
}

// GetTaskEvents — Redis does not natively support audit logs.
// A real implementation would use a Redis Stream or separate List per task.
func (q *RedisQueue) GetTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error) {
	return nil, domain.ErrNotImplemented
}

// Heartbeat updates worker last_seen in a Redis Hash.
func (q *RedisQueue) Heartbeat(ctx context.Context, workerID string) error {
	return domain.ErrNotImplemented
}

// RenewLease updates lease_expires_at in the task hash (with Lua for atomicity).
func (q *RedisQueue) RenewLease(ctx context.Context, taskID string, workerID string, leaseTimeout time.Duration) error {
	return domain.ErrNotImplemented
}

// Stats returns task counts by status. Redis can compute this via a SCAN of
// task:* keys, but it is O(n); a real implementation would maintain counters.
func (q *RedisQueue) Stats(ctx context.Context) (map[domain.TaskStatus]int64, error) {
	return nil, domain.ErrNotImplemented
}

// --- Design notes for full Redis implementation ---

// ClaimTasks Lua script (pseudocode):
//
// local ready = redis.call('LRANGE', 'queue:ready', 0, limit-1)
// local delayed = redis.call('ZRANGEBYSCORE', 'queue:delayed', '-inf', ARGV[1], 'LIMIT', 0, limit)
// for _, id in ipairs(ready) do
//     local task = redis.call('HGET', 'tasks:'..id, 'data')
//     -- update status, leased_by, lease_expires_at
//     redis.call('HSET', 'tasks:'..id, 'data', updated_json)
//     redis.call('LREM', 'queue:ready', 0, id)
// end
// return claimed_tasks

// PostgreSQL vs Redis design comparison:
//
// | Concern               | PostgreSQL                          | Redis                              |
// |-----------------------|-------------------------------------|------------------------------------|
// | Claim atomicity       | FOR UPDATE SKIP LOCKED in tx       | Lua script (single-threaded)      |
// | Query/filter          | Rich SQL (WHERE, ORDER BY)         | Key-based only, scan is O(n)      |
// | Delayed retry         | next_run_at column + index         | ZSet sorted by score              |
// | Task metadata         | Single tasks table                 | Hash per task key                 |
// | Audit/events          | task_events table                  | Stream or separate List           |
// | Operations overhead   | One DB service                     | Additional Redis service          |
// | MVP fit               | Excellent for state-machine demos  | Better for high-throughput later  |
