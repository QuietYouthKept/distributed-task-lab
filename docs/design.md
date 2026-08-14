# Design Notes — distributed-task-lab

## System Architecture

The system separates concerns cleanly:

```
API Server  ←─  Receives task submissions, returns status.
                Never executes heavy work. Stateless (can scale horizontally).

PostgreSQL  ←─  Single source of truth for task state.
                Handles coordination via transactions, not a message broker.

Worker Pool ←─  Stateless, independently scalable.
                Claims tasks atomically, executes them, reports results.
```

### Why the API Server Doesn't Execute Tasks

If the API server executed tasks synchronously:
- Request latency would spike (RAG ingestion can take minutes)
- Timeouts would kill in-progress work
- Scaling would couple API throughput to task throughput

By separating submission from execution, the API stays fast and workers
scale independently.

## Task State Machine

```
pending → running → success
                 → failed (non-retryable or max_attempts hit)
                 → pending (retry: next_run_at delayed)
                 → dlq    (attempt >= max_attempts)
```

All transitions happen via the Queue interface methods (`ClaimTasks`,
`MarkSuccess`, `MarkFailure`), each with ownership verification.

## At-Least-Once Semantics

This system explicitly chooses **at-least-once** over exactly-once. Here's why:

1. **Worker crashes are inevitable.** If a worker dies mid-execution, we can't
   know whether side effects happened. The safe choice is to re-execute.
2. **Exactly-once requires coordination** (two-phase commit, idempotency
   tokens at the infrastructure level) that adds significant complexity.
3. **Idempotency at the application level** is simpler and more robust.

### How At-Least-Once Happens

```
Worker A claims task T
Worker A starts executing T (side effects begin)
Worker A crashes ─────────────────────── [lease expires]
Worker B claims task T (lease_expires_at < now)
Worker B executes T again
```

Both Worker A (partially) and Worker B (fully) executed T. The application
handler must handle this by being idempotent.

### Idempotency Pattern

For RAG ingestion, use a composite key:

```
(document_id, version, index_name)
```

Before writing to the index, check if this version is already indexed.
If yes, skip. If no, process and record the version.

The `idempotency_key` field on the Task supports this pattern at the
submission level — clients can submit with the same key to avoid
duplicate task creation.

## Lease Mechanism

### Why Leases?

Without leases, a naive approach would be:

```sql
-- Worker reads pending tasks
SELECT * FROM tasks WHERE status = 'pending' LIMIT 5;
-- Worker updates them to running
UPDATE tasks SET status = 'running' WHERE id IN (...);
```

This has a **race condition**: two workers could read the same pending tasks
before either updates them. Row-level locks prevent this.

### The Correct Approach

```sql
BEGIN;
-- Select and lock candidate tasks
SELECT * FROM tasks
WHERE (status = 'pending' AND next_run_at <= now())
   OR (status = 'running' AND lease_expires_at < now())
ORDER BY priority DESC, created_at ASC
LIMIT $batch_size
FOR UPDATE SKIP LOCKED;

-- Update each claimed task
UPDATE tasks SET
    status = 'running',
    leased_by = $worker_id,
    lease_expires_at = now() + $lease_timeout,
    started_at = COALESCE(started_at, now()),
    attempt = attempt + 1
WHERE id = $task_id;
COMMIT;
```

`FOR UPDATE SKIP LOCKED` is the key: it locks the selected rows and skips
any rows that are already locked by another transaction. This guarantees
**no two workers claim the same task**.

### Ownership Check on Completion

When a worker finishes a task, it must prove it still owns the lease:

```sql
UPDATE tasks SET status = 'success', ...
WHERE id = $task_id
  AND status = 'running'
  AND leased_by = $worker_id;
```

If this updates 0 rows, the worker lost the lease (another worker claimed it,
or the lease expired). The worker's result is silently discarded — this is
correct under at-least-once.

## Task Timeout vs. Lease Timeout

| Concept | Meaning | Example |
|---------|---------|---------|
| `timeout_seconds` | Maximum time a task handler may run | 60s for RAG ingestion |
| `lease_timeout` | How long a worker owns a task before others can claim it | 90s (task timeout + buffer) |

**Rule of thumb:** `lease_timeout >= timeout_seconds + buffer`

Without heartbeat (MVP), the buffer prevents premature re-claiming. If a task
truly hangs, the lease eventually expires and another worker picks it up.

### Timeout Enforcement (Phase 3)

The worker uses the task's `timeout_seconds` field for execution timeout:

```go
taskTimeout := time.Duration(job.TimeoutSeconds) * time.Second
if taskTimeout <= 0 || taskTimeout > p.leaseTimeout {
    taskTimeout = p.leaseTimeout - time.Second
}
ctx, cancel := context.WithTimeout(ctx, taskTimeout)
defer cancel()
```

If the handler respects `ctx.Done()`, the task is cancelled cleanly.
If it doesn't (CPU-bound loop without context checks), the goroutine leaks
but the lease expires and another worker handles the retry.

Timeout errors are detected via `context.DeadlineExceeded` and recorded
with `error_type = "timeout"` for observability.

## Retry Strategy (Phase 3)

Phase 3 implements **fixed-interval retry**:

- On failure: `attempt` (already incremented by ClaimTasks) is checked against `max_attempts`
- If `attempt < max_attempts`: status → `pending`, `next_run_at = now + retry_delay_seconds`, lease cleared
- If `attempt >= max_attempts`: status → `failed` (Phase 4 will change this to `dlq`)
- `last_error` records the failure message

### Why Fixed-Interval in MVP

Fixed-interval retry is simple, predictable, and easy to verify in demos.
It clearly shows the retry mechanism without the complexity of backoff math.

### Future: Exponential Backoff + Jitter

```
delay = base_delay * 2^(attempt-1) + random_jitter
```

This prevents thundering herd when many tasks fail simultaneously.

### Future: Retryable vs. Non-Retryable Errors

```go
type TaskFailure struct {
    ErrorType string // "temporary", "permanent", "timeout"
    Retryable bool
}
```

Non-retryable errors (e.g., invalid payload) would skip retry and go directly
to DLQ without consuming retry attempts.

## Dead Letter Queue (DLQ)

The DLQ is not a separate table — it's a status (`dlq`) on the `tasks` table.
This keeps the schema simple and allows unified querying.

### When Tasks Enter DLQ

When `MarkFailure` is called and `attempt >= max_attempts`:

```sql
UPDATE tasks SET status = 'dlq',
    dlq_reason = 'max attempts exceeded: ' || $last_error,
    last_error = $last_error,
    finished_at = now(),
    leased_by = NULL, lease_expires_at = NULL
WHERE id = $task_id AND status = 'running' AND leased_by = $worker_id;
```

### DLQ Tasks Are Not Claimable

`ClaimTasks` only selects `status = 'pending'` or `status = 'running'` with
expired lease — `dlq` is naturally excluded.

### Future: Requeue from DLQ

```bash
POST /tasks/{id}/requeue
# Resets: attempt=0, status=pending, next_run_at=now, dlq_reason=NULL
```

This would allow operators to manually retry DLQ tasks after fixing the
underlying issue (e.g., correcting a bad payload).

## Heartbeat (Phase 6)

Workers send periodic heartbeats to a `workers` table:

```sql
INSERT INTO workers (worker_id, hostname, last_seen_at, status)
VALUES ($1, $2, now(), 'active')
ON CONFLICT (worker_id) DO UPDATE SET last_seen_at = now();
```

The heartbeat runs every `lease_timeout / 3` in a dedicated goroutine.

### Lease Renewal

For long-running tasks, workers can extend their lease:

```sql
UPDATE tasks SET lease_expires_at = now() + $lease_timeout
WHERE id = $task_id AND status = 'running' AND leased_by = $worker_id;
```

If renewal fails (0 rows), the worker lost the lease and must stop the task.

### Stale Worker Detection

```sql
SELECT worker_id FROM workers
WHERE last_seen_at < now() - $threshold AND status = 'active';
```

Stale workers can be marked inactive, and their leased tasks (with expired
leases) are automatically reclaimed by other workers.

## Redis Backend (Phase 6 Skeleton)

A Redis implementation of the `Queue` interface is provided as a design
skeleton at `internal/storage/redis/redis_queue.go`. It returns
`ErrNotImplemented` for all methods but documents the intended key structure
and Lua scripts.

### Key Structure

```
queue:ready          — List of task IDs ready for claiming
queue:delayed        — ZSet of task IDs with next_run_at as score
tasks:{task_id}      — Hash storing full task JSON
queue:dlq            — List of DLQ task IDs
```

### Atomic Claim via Lua

```lua
local ready = redis.call('LRANGE', 'queue:ready', 0, limit-1)
local delayed = redis.call('ZRANGEBYSCORE', 'queue:delayed', '-inf', now, 'LIMIT', 0, limit)
-- For each ID: update task status, set leased_by, lease_expires_at
-- Return claimed tasks
```

Lua scripts execute atomically in Redis, providing the same safety guarantee
as PostgreSQL's `FOR UPDATE SKIP LOCKED` but with different tradeoffs.

## Database Design

### Why PostgreSQL Instead of Redis?

| Aspect | PostgreSQL | Redis |
|--------|-----------|-------|
| **Transactions** | Full ACID, row-level locks | Limited (Lua scripts) |
| **Queries** | Rich SQL filtering | Key-based only |
| **Durability** | WAL, point-in-time recovery | Configurable (RDB/AOF) |
| **Operations** | Single service to manage | Additional infrastructure |
| **MVP fit** | Excellent for state-machine demos | Better for high-throughput later |

PostgreSQL lets us demonstrate transaction correctness, conditional updates,
and rich querying — exactly what we want to show.

### Why One Table?

A single `tasks` table keeps the MVP simple. Phase 2 can add:

- `task_events` — audit log of state transitions
- `workers` — worker registry and heartbeats
- `task_locks` — distributed lock tracking (if we need finer-grained locks)

### Key Indexes

```sql
-- For worker claim queries (the hottest path)
CREATE INDEX idx_tasks_status_next_run ON tasks (status, next_run_at, priority);

-- For stale-lease detection
CREATE INDEX idx_tasks_status_lease ON tasks (status, lease_expires_at);

-- For API filtering
CREATE INDEX idx_tasks_type ON tasks (type);
CREATE INDEX idx_tasks_created_at ON tasks (created_at);
```

## Preventing Split-Brain

"Split-brain" here means multiple workers believing they own the same task.
Three layers prevent this:

### Layer 1: Atomic Claim

The `FOR UPDATE SKIP LOCKED` in a transaction ensures exactly one worker
acquires each task row. This is enforced by PostgreSQL's MVCC.

### Layer 2: Ownership Check + Fencing Token

`MarkSuccess` and `MarkFailure` verify `leased_by = $worker_id AND status = 'running'
AND lease_version = $version`. `lease_version` is a fencing token incremented on every
claim: if a worker's lease expires and the task is reclaimed (bumping the token),
the stale worker's write-back fails even if it still holds the same worker id. If
another worker claimed the task, the original worker's update affects 0 rows.

### Layer 3: Idempotent Handlers

Even if layers 1 and 2 fail (e.g., a bug), idempotent handlers prevent
duplicate side effects. This is the application-level safety net.

## Concurrency Model

### Worker Pool

```
┌─────────────────────────────────┐
│         Worker Process          │
│                                 │
│  claimLoop() ──► jobs channel   │
│                                 │
│  executeLoop(0) ◄──             │
│  executeLoop(1) ◄──             │
│  executeLoop(2) ◄──             │
│  executeLoop(3) ◄──             │
│                                 │
│  buffered channel (cap: 100)   │
└─────────────────────────────────┘
```

- **claimLoop:** Single goroutine, polls PostgreSQL, pushes to channel.
- **executeLoop:** N goroutines, pull from channel, execute tasks concurrently.
- **Channel buffer** (100) decouples claiming from execution.

This model is simple but effective. Phase 2 could add dynamic scaling based
on queue depth.

## Graceful Shutdown

1. SIGTERM received
2. `cancel()` called on pool context
3. claimLoop stops polling
4. executeLoops finish in-flight tasks (up to lease timeout)
5. `sync.WaitGroup` ensures all goroutines exit
6. Process exits

In-flight tasks that don't finish before the parent context deadline are
abandoned — their leases expire and other workers pick them up.

## Queue Governance

Each task belongs to a named queue (`default` by default). The `queues` table
holds per-queue policy so different workload classes can be governed
independently (e.g. a RAG-ingestion queue that hammers a vector DB, and a
low-priority benchmark queue).

- `max_concurrency` caps concurrently running tasks in the queue (0 = unlimited).
  Enforced inside the claim transaction with a per-task re-check so a batch claim
  cannot overshoot the cap.
- `paused` lets operators stop a queue from claiming new work without losing tasks.
- `rate_limit_per_second` is reserved for a future token-bucket rate limiter.

## Data Lifecycle and Retention

The `tasks` table is the active working set. Over time, completed and DLQ'd rows
accumulate. Production strategy:

1. **Separate active vs. historical data.** Keep only `pending`/`running` rows in
   the hot path (covered by `idx_tasks_queue_status`), and move terminal rows
   (`success`, `failed`, `dlq`) to an archive table (or a partitioned table) on a
   schedule.
2. **Retain `task_events` for a bounded window** (e.g. 30 days), then purge. Events
   are append-only audit data and grow without bound.
3. **Cap payload size** at the API boundary to avoid `JSONB` bloat; large artifacts
   should be stored as a URI, not inlined.

These are documented as a strategy rather than implemented in the MVP; see the
roadmap under "Archive / retention".
