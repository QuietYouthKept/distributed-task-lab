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
