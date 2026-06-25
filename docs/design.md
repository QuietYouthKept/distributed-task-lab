# Design Notes — distributed-task-lab

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

### Timeout Enforcement

The worker creates a context with timeout:

```go
ctx, cancel := context.WithTimeout(ctx, leaseTimeout - buffer)
defer cancel()
result, err := handler.Handle(ctx, taskType, payload)
```

If the handler respects `ctx.Done()`, the task is cancelled cleanly.
If it doesn't (CPU-bound loop without context checks), the goroutine leaks
but the lease expires and another worker handles the retry.

## Retry Strategy

MVP uses **fixed-interval retry**:

- `attempt` increments on each failure
- `next_run_at = now + retry_delay_seconds`
- Status returns to `pending`
- When `attempt >= max_attempts` → DLQ

### Phase 2: Exponential Backoff

```
delay = base_delay * 2^(attempt-1) + random_jitter
```

This prevents thundering herd when many tasks fail simultaneously.

### Retryable vs. Non-Retryable Errors

MVP treats all errors as retryable. Phase 2 can add:

```go
type TaskFailure struct {
    ErrorType string // "temporary", "permanent", "timeout"
    Retryable bool
}
```

Non-retryable errors (e.g., invalid payload) go directly to DLQ without
consuming retry attempts.

## Dead Letter Queue (DLQ)

The DLQ is not a separate table — it's a status (`dlq`) on the `tasks` table.
This keeps the schema simple and allows unified querying.

### Querying DLQ

```bash
# Via status filter
GET /tasks?status=dlq

# Via convenience endpoint
GET /dlq
```

### When Tasks Enter DLQ

1. `attempt >= max_attempts` after repeated failures
2. Non-retryable error (Phase 2)
3. Manual move (Phase 2 `POST /tasks/{id}/requeue` reverse operation)

### Phase 2: Requeue from DLQ

```bash
POST /tasks/{id}/requeue
# Resets attempt=0, status=pending, next_run_at=now
```

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

### Layer 2: Ownership Check

`MarkSuccess` and `MarkFailure` verify `leased_by = $worker_id AND status = 'running'`.
If another worker claimed the task, the original worker's update affects 0 rows.

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
