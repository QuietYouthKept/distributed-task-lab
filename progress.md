# Progress Log — distributed-task-lab

## Phase 1: PostgreSQL-backed Task API ✅

### Date: 2025-06-25

### What was done

- [x] PostgreSQL tasks table with full schema (id, type, status, payload, result, priority, max_attempts, attempt, timeout_seconds, retry_delay_seconds, leased_by, lease_expires_at, next_run_at, created_at, updated_at, started_at, finished_at, last_error, dlq_reason, idempotency_key)
- [x] Database migrations in `internal/storage/postgres/migrations/001_create_tasks.sql`
- [x] Domain models split into:
  - `internal/domain/status.go` — TaskStatus (pending/running/success/failed/dlq), TaskType (rag_ingestion/eval/benchmark), validation maps
  - `internal/domain/payload.go` — TaskPayloadEnvelope, TaskResult, TaskFailure
  - `internal/domain/task.go` — Task struct, TaskFilter, RetryPolicy
  - `internal/domain/errors.go` — Domain errors including ErrNotImplemented
- [x] Queue interface in `internal/queue/queue.go` with Enqueue, GetTask, ListTasks, ClaimTasks, MarkSuccess, MarkFailure, ListDLQ
- [x] PostgreSQL implementation in `internal/storage/postgres/task_repository.go`:
  - Enqueue — creates task with status=pending, attempt=0
  - GetTask — retrieves task by ID, returns nil if not found
  - ListTasks — lists with status/type filter, limit/offset
  - ClaimTasks — returns ErrNotImplemented (Phase 2)
  - MarkSuccess — returns ErrNotImplemented (Phase 2)
  - MarkFailure — returns ErrNotImplemented (Phase 2)
  - ListDLQ — returns ErrNotImplemented (Phase 2)
- [x] Application layer in `internal/application/task_service.go` — CreateTask, GetTask, ListTasks, ListDLQ
- [x] HTTP API:
  - `POST /tasks` — create task, default type rag_ingestion, status=pending
  - `GET /tasks/{id}` — get single task
  - `GET /tasks` — list tasks with ?status= and ?type= filters
  - `GET /dlq` — returns not implemented
  - `GET /health` — health check
- [x] Docker Compose: PostgreSQL + API (no Worker in Phase 1)
- [x] Makefile targets: up, down, build, api, migrate, submit-rag-task, list-tasks, test, logs, clean, dev
- [x] Example payloads: rag_ingestion.json, eval.json, benchmark.json, retry.json, dlq.json, timeout.json
- [x] Tests: domain validation, payload parsing, PostgreSQL CRUD (integration tests auto-skip without DB)
- [x] README updated with Quick Start for Phase 1

### Verification

```bash
# Start the system
make up

# Submit a RAG ingestion task
make submit-rag-task
# → {"id":"...", "status":"pending"}

# List all tasks
make list-tasks
# → [{"id":"...", "status":"pending", "type":"rag_ingestion", ...}]

# Get task by ID
curl http://localhost:8080/tasks/{id}
# → {"id":"...", "status":"pending", ...}

# Run tests
make test
```

### Not in Phase 1 (explicitly deferred to Phase 2+)

- Worker process (cmd/worker, internal/worker/)
- ClaimTasks / MarkSuccess / MarkFailure implementation
- Lease mechanism
- Retry logic
- Timeout enforcement
- DLQ processing
- Multi-worker concurrency
- Heartbeat
- Prometheus metrics

### Next: Phase 2 — Worker Claim + Lease

---

## Phase 2: Worker Claim + PostgreSQL Lease ✅

### Date: 2025-06-25

### What was done

- [x] Worker process implementation:
  - `cmd/worker/main.go` — entry point with config loading, DB init, graceful shutdown
  - `internal/worker/pool.go` — goroutine pool with claim loop + execute loop
  - `internal/worker/handlers/rag_ingestion.go` — mock handler with simulation modes
- [x] ClaimTasks fully implemented:
  - Atomic transaction with `FOR UPDATE SKIP LOCKED`
  - Sets `leased_by`, `lease_expires_at`, `status = running`, `attempt += 1`
  - Claims both `pending` tasks and `running` tasks with expired leases
- [x] MarkSuccess with ownership check (`WHERE leased_by = $workerID AND status = 'running'`)
- [x] MarkFailure with ownership check (simple fail — no retry/DLQ in Phase 2)
- [x] Docker Compose: worker service added, supports `--scale worker=3`
- [x] Makefile targets: worker, submit-multi, watch-tasks, demo-claim, logs-worker
- [x] Tests: ClaimTasks, MarkSuccess, MarkSuccess-wrong-worker, MarkFailure, ReclaimExpiredLease
- [x] README updated with multi-worker demo instructions and Phase status table

### Verification

```bash
# Start with 3 workers
docker compose -f deployments/docker-compose.yml up -d --scale worker=3

# Submit multiple tasks
make submit-multi

# Watch which worker claims each task
make watch-tasks
# → Tasks transition: pending → running (with leased_by set) → success

# Check worker logs
make logs-worker
# → See worker_id and claimed task IDs

# Run tests
make test
```

### Key design points proven

1. **Atomic claim**: `FOR UPDATE SKIP LOCKED` prevents two workers claiming the same task
2. **Ownership check**: `MarkSuccess`/`MarkFailure` verify `leased_by` before updating
3. **Lease recovery**: Expired leases allow other workers to reclaim orphaned tasks
4. **At-least-once semantics**: Documented in README — handlers must be idempotent

### Not in Phase 2 (explicitly deferred)

- Retry logic (Phase 3)
- Timeout enforcement (Phase 3)
- DLQ processing (Phase 4)
- Heartbeat / stale worker detection (Phase 2 future)
- Real RAG ingestion
- Prometheus metrics

### Next: Phase 3 — Retry + Timeout

---

## Phase 3: Retry + Task Timeout ✅

### Date: 2025-06-25

### What was done

- [x] Retry logic in MarkFailure:
  - If `attempt < max_attempts`: status → pending, `next_run_at = now + retry_delay_seconds`, lease cleared
  - If `attempt >= max_attempts`: status → failed (final; Phase 4 will change to DLQ)
  - Ownership check preserved for both paths
- [x] Task timeout enforcement:
  - Worker passes `TimeutSeconds` through job channel
  - `executeTask` uses `time.Duration(task.TimeoutSeconds)` for context timeout
  - Timeout detection via `context.DeadlineExceeded` → error type "timeout"
  - Lease timeout still serves as upper bound for safety
- [x] Mock handler updated:
  - Accepts `attempt` parameter from worker
  - New simulation params: `mock_failures_before_success`, `mock_always_fail`, `mock_duration_ms`
  - Backward-compatible: `simulate` shortcuts still work
- [x] Example payloads: retry.json (mock_failures_before_success=1), timeout.json (mock_duration_ms=10s, timeout=2s)
- [x] Makefile: demo-retry, demo-timeout targets
- [x] Tests: retry flow, max attempts exhausted, delayed reclaim, wrong worker ownership, attempt increment
- [x] README: Phase status table updated

### Verification

```bash
# Retry demo
make demo-retry
# → Task fails on attempt 1, goes back to pending, retried after delay, succeeds on attempt 2

# Timeout demo
make demo-timeout
# → Task exceeds timeout_seconds, marked as failed with error_type=timeout, retried

# Run tests
make test
```

### Key design points

1. **Fixed-interval retry**: `next_run_at = now + retry_delay_seconds` — simple, predictable
2. **Attempt tracking**: `attempt` incremented on each claim (in ClaimTasks), not on each failure
3. **Task timeout vs Lease timeout**: task timeout is per-execution; lease timeout is ownership duration
4. **Timout safety**: `taskTimeout = min(timeout_seconds, lease_timeout - 1s)`

### Not in Phase 3 (deferred)

- Exponential backoff + jitter (Phase 2 future / README roadmap)
- Retryable vs non-retryable error classification
- DLQ processing (Phase 4)
- Heartbeat / stale worker detection

### Next: Phase 4 — DLQ + Query

---

## Phase 4: Dead Letter Queue ✅

### Date: 2025-06-25

### What was done

- [x] MarkFailure: when `attempt >= max_attempts`, status → `dlq` with `dlq_reason = "max attempts exceeded: <error>"`
- [x] ListDLQ un-stubbed: uses `ListTasks` with status=dlq filter
- [x] ClaimTasks confirmed to exclude DLQ tasks (WHERE status=pending OR status=running)
- [x] API: `GET /tasks?status=dlq` and `GET /dlq` both return DLQ tasks
- [x] Example payload: dlq.json (mock_always_fail=true, max_attempts=2, retry_delay=2s)
- [x] Makefile: demo-dlq target showing pending→running→pending→running→dlq flow
- [x] Tests: DLQ not claimable, ListDLQ returns DLQ tasks, status filter works
- [x] README: Phase status table updated (DLQ → ✅ Phase 4)
- [x] progress.md: Phase 4 completion recorded

### Verification

```bash
make demo-dlq
# → Task submitted with mock_always_fail=true, max_attempts=2
# → Attempt 1: fails → retry to pending
# → Attempt 2: fails → DLQ
# → curl /tasks?status=dlq shows the task with dlq_reason

# Direct API test:
curl "http://localhost:8080/tasks?status=dlq"
# or
curl http://localhost:8080/dlq
```

### Not in Phase 4 (deferred)

- Manual requeue (`POST /tasks/{id}/requeue`)
- Cancel API
- DLQ Web UI
- Alerting system
- Complex error classification

### Next: Phase 5 — Makefile + Docker Demo + README Polish

---

## Phase 5: Docker Demo + README Polish ✅

### Date: 2025-06-25

### What was done

- [x] `make demo` — comprehensive 7-step demo: start → success → retry → DLQ → stats
- [x] `make fmt` — run `go fmt ./...`
- [x] `GET /stats` endpoint — returns task counts by status (pending/running/success/failed/dlq)
- [x] README fully rewritten:
  - Project overview → Why → Quick Demo → Architecture → Key Mechanisms
  - Task Lifecycle diagram → API Reference table → Worker & Lease Design
  - Retry/Timeout/DLQ explained → At-Least-Once → Non-Goals → Roadmap
- [x] DESIGN.md enhanced:
  - System Architecture section (API/PostgreSQL/Worker separation)
  - Task State Machine section
  - Heartbeat Phase 2 design (reserved)
  - Why Redis is a future extension (comparison table)
  - Updated DLQ section with implementation details
- [x] Makefile: all recommended targets present (up/down/api/worker/demo/demo-claim/demo-retry/demo-timeout/demo-dlq/logs/test/fmt)

### Verification

```bash
make demo
# → 7-step demo: success → retry → DLQ → stats

make test && make fmt
# → All tests pass, code formatted

curl http://localhost:8080/stats
# → {"pending":0,"running":0,"success":N,"failed":0,"dlq":0}
```

### MVP Complete 🎉

All 5 milestones delivered. The project demonstrates:

- Go backend infrastructure
- PostgreSQL transaction correctness (`FOR UPDATE SKIP LOCKED`)
- Distributed worker lease with ownership checks
- At-least-once task execution with idempotency guidance
- Fixed-interval retry + task timeout
- Dead Letter Queue
- AI-style background job scenario (RAG ingestion mock)

### Next Steps (beyond MVP)

- Exponential backoff with jitter
- Worker heartbeat
- Redis backend
- `task_events` audit table
- Manual DLQ requeue
- Prometheus metrics
