# distributed-task-lab

A small distributed background job system built in Go, designed to demonstrate
task state machines, PostgreSQL-backed leasing, retry, timeout, and Dead Letter
Queue (DLQ) mechanisms through AI-style background jobs such as RAG ingestion,
eval, and benchmark tasks.

## Why This Project

Many AI applications shouldn't run heavy work synchronously inside API requests —
things like document ingestion, model evaluation, and benchmarks. This project
shows how to build a background task system where:

- The **API server** only submits tasks and queries status — no heavy lifting.
- **Workers** independently claim and execute tasks.
- **PostgreSQL** acts as the source of truth, using transactions and row-level
  locks to safely coordinate multiple workers (no Redis required).

The system uses **at-least-once** semantics: tasks may be executed more than
once if a worker crashes. Task handlers must be designed to be idempotent.

## Quick Demo

```bash
# Prerequisites: Docker, curl, jq

make demo              # The full story: success → retry → DLQ
make demo-claim        # Multi-worker claiming
make demo-retry        # Automatic retry on failure
make demo-dlq          # Dead Letter Queue after max retries

# Production-hardening demos
make demo-failure-lab  # Worker crash → lease expiry → fencing-token recovery
make demo-rate-limit   # Per-queue token-bucket rate limiting
make demo-observe      # Prometheus + Grafana dashboard
make loadgen           # Throughput + claim-latency + duplicate-claim report
```

## Failure Lab

`make demo-failure-lab` spins up a dedicated Postgres and drives a task through a
worker crash and recovery, showing the fencing token in action:

```text
1. task submitted: failure-lab-...
2. worker-a claimed: lease_version=1, attempt=1
3. worker-a crashed (no heartbeat) — lease expires in ~2s
4. worker-b reclaimed: lease_version=2, attempt=2
5. stale write-back from worker-a (lease_version=1): REJECTED (task is not claimed by this worker)
6. worker-b completed the task

--- task_events timeline ---
  ...  submitted  worker=          task submitted
  ...  claimed    worker=worker-a  task claimed
  ...  claimed    worker=worker-b  task claimed
  ...  succeeded  worker=worker-b  task completed

final status: success
```

## Architecture

```
Client / Demo Script
        │
        ▼
   API Server          ───   POST /tasks, GET /tasks/{id}, GET /tasks, GET /stats
        │
        ▼
   PostgreSQL           ───   Task state center (tasks table)
        ▲
        │
  ┌─────┴─────┐
  │           │
Worker 1   Worker 2   ───   Independent goroutine pools
```

### Key Mechanisms

| Mechanism | Description | Status |
|-----------|-------------|--------|
| **Lease** | Workers claim tasks in a DB transaction with `FOR UPDATE SKIP LOCKED`. Each claim sets `leased_by` and `lease_expires_at`. If a worker crashes, the lease expires and another worker picks up the task. | ✅ |
| **Retry** | Failed tasks increment `attempt` and are re-scheduled with `next_run_at = now + retry_delay`. Fixed-interval retry. | ✅ |
| **Timeout** | Each task has a `timeout_seconds`. If execution exceeds it, the task is marked as failed and retried. | ✅ |
| **DLQ** | When `attempt >= max_attempts`, the task moves to DLQ (`status = 'dlq'`) with the failure reason recorded. | ✅ |

## Task Lifecycle

```
              submit
                │
                ▼
            pending
                │
      worker acquires lease
                │
                ▼
            running
           /   │    \
          /    │     \
     success  error  timeout
        │      │       │
        ▼      ▼       ▼
     done   retry?   retry?
               │       │
           yes │       │ yes
               ▼       ▼
           pending  pending
               \      /
                \    /
            max retries exceeded
                   │
                   ▼
                  dlq
```

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/tasks` | Submit a new task |
| `GET` | `/tasks` | List tasks (?status=, ?type= filters) |
| `GET` | `/tasks/{id}` | Get a single task |
| `GET` | `/tasks/{id}/events` | Get task audit event log |
| `POST` | `/tasks/{id}/requeue` | Reset a DLQ task to pending |
| `POST` | `/tasks/{id}/cancel` | Cancel a pending or running task |
| `GET` | `/tasks?status=dlq` | List DLQ tasks |
| `GET` | `/dlq` | Same as above |
| `GET` | `/stats` | Task counts by status (JSON) |
| `GET` | `/metrics` | Prometheus-format metrics (per-queue depth, oldest-pending age) |
| `GET` | `/queues` | List queues with governance config |
| `PUT` | `/queues/{name}` | Set a queue's concurrency / rate limit |
| `POST` | `/queues/{name}/pause` | Pause a queue |
| `POST` | `/queues/{name}/resume` | Resume a queue |
| `GET` | `/health` | Health check |

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) & Docker Compose
- [Go](https://go.dev/dl/) 1.22+ (for local development)
- `curl` and `jq`

### Start Everything

```bash
make up

# Or with 3 workers:
docker compose -f deployments/docker-compose.yml up -d --scale worker=3
```

### Submit and Watch Tasks

```bash
make submit-rag-task     # Submit a single task
make submit-multi        # Submit 5 tasks at once
make list-tasks          # List all tasks
make watch-tasks         # Real-time task state watch
```

### Quick API Test

```bash
# Submit
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"type":"rag_ingestion","payload":{"version":"1","kind":"rag_ingestion","data":{"document_id":"doc-001","index_name":"demo-index"}}}'

# Check
curl http://localhost:8080/tasks/<task-id>

# Stats
curl http://localhost:8080/stats
```

## Worker and Lease Design

### How Workers Claim Tasks

Workers use a PostgreSQL transaction with row-level locking:

```sql
BEGIN;
SELECT * FROM tasks
WHERE (status = 'pending' AND next_run_at <= now())
   OR (status = 'running' AND lease_expires_at < now())
ORDER BY priority DESC, created_at ASC
LIMIT $batch_size
FOR UPDATE SKIP LOCKED;

UPDATE tasks SET status = 'running', leased_by = $worker_id,
       lease_expires_at = now() + $lease_timeout, attempt = attempt + 1
WHERE id = $task_id;
COMMIT;
```

**`FOR UPDATE SKIP LOCKED`** guarantees no two workers claim the same task.
The second `WHERE` clause handles **worker crash recovery**: if Worker A dies,
its lease expires and Worker B can reclaim the task.

### Ownership Check

When a worker finishes (success or failure), it must prove ownership:

```sql
UPDATE tasks SET status = 'success', ... 
WHERE id = $task_id AND status = 'running' AND leased_by = $worker_id;
```

If 0 rows affected → the worker lost the lease (another worker reclaimed it).
This prevents split-brain: a zombie worker can't corrupt task state.

Each claim also increments a `lease_version` **fencing token**, and
`MarkSuccess`/`MarkFailure` must present the matching token. This defends against
a stale worker from an earlier lease generation writing back results — the
classic distributed-systems fencing-token pattern.

## Retry, Timeout, and DLQ

### Retry (Exponential Backoff + Jitter)

- On failure: `status → pending`, `next_run_at = now + backoff(attempt)`, lease cleared
- Backoff grows exponentially: `initial_delay × multiplier^(attempt-1)`, capped by
  `retry_max_delay_seconds`, with optional full jitter
- `attempt` is incremented on each claim (in `ClaimTasks`), not on each failure
- **Non-retryable** errors (validation, bad payload, unsupported type) go straight
  to the DLQ regardless of attempts left
- When `attempt >= max_attempts`: task enters DLQ

Task fields: `retry_delay_seconds` (initial), `retry_backoff_multiplier` (default 2.0),
`retry_max_delay_seconds` (cap), `retry_jitter` (default true).

### Task Timeout vs Lease Timeout

| Concept | Meaning | Example |
|---------|---------|---------|
| `timeout_seconds` | Max time a handler may run | 60s |
| `lease_timeout` | How long a worker owns a task | 90s |

**Rule:** `lease_timeout >= timeout_seconds + buffer` — without heartbeat,
the buffer prevents premature re-claiming.

### DLQ (Dead Letter Queue)

- Tasks that exhaust all retry attempts enter `status = 'dlq'`
- Non-retryable failures (validation/bad payload) enter `status = 'dlq'` immediately
- `dlq_reason` records "max attempts exceeded: <last error>" or "non-retryable error: <last error>"
- DLQ tasks are **never claimed** by workers (excluded by the WHERE clause)
- Query via `GET /tasks?status=dlq` or `GET /dlq`
- Manual requeue via `POST /tasks/{id}/requeue`

## Idempotency and Queue Governance

### Idempotent Submission

A task may carry an `idempotency_key`. Enqueuing with an existing key returns the
original task instead of creating a duplicate — backed by a `UNIQUE` index and
`INSERT ... ON CONFLICT DO NOTHING`. Handlers must still be idempotent because
the system is at-least-once.

### Per-Queue Concurrency and Pause

Each task belongs to a `queue` (default: `default`). The `queues` table holds
per-queue governance policy:

- `max_concurrency` — cap on concurrently running tasks in the queue (0 = unlimited)
- `paused` — pause/resume a queue so workers stop claiming its tasks
- `rate_limit_per_second` — reserved for rate limiting

Manage queues via `GET /queues`, `PUT /queues/{name}`, `POST /queues/{name}/pause`,
and `POST /queues/{name}/resume`.

## Real RAG Ingestion Workload

When a `rag_ingestion` task carries a `source_path`, the worker runs a lightweight
but real pipeline instead of a mock: read a markdown file, chunk it by paragraph,
generate deterministic SHA-256 embeddings, and write a JSONL artifact to
`artifacts/index/`. Re-ingesting the same `document_id + version + chunking_profile
+ content` reuses the existing artifact (`idempotent_reused: true`) rather than
rewriting it.

```bash
# Submit a real ingestion task (examples/docs/rag-pipeline.md)
curl -X POST http://localhost:8080/tasks -H "Content-Type: application/json" \
  -d @examples/payloads/rag_ingestion_real.json
```

No external embedding API or vector DB is required — the deterministic hash keeps
the demo self-contained and idempotency easy to reason about.

## Observability

`GET /metrics` exposes Prometheus metrics: `tasklab_tasks_total{status}`,
`tasklab_queue_depth{queue,status}`, and `tasklab_oldest_pending_age_seconds`. A
ready-made Grafana stack (`deployments/observability/`) visualizes queue depth,
oldest pending age, and per-status task counts:

```bash
make demo-observe
# Grafana  : http://localhost:3000 (dashboard: distributed-task-lab Overview)
# Prometheus: http://localhost:9090
```

## At-Least-Once Semantics

This system guarantees **at-least-once** delivery: every task will be attempted
at least once, but may execute more than once if:

- A worker crashes after starting but before marking success/failure
- A lease expires and another worker claims the same task
- Network issues cause retry timeouts

**Task handlers must be idempotent.** For RAG ingestion, check
`document_id + version` before writing to the index.

## Project Structure

```
cmd/api/              API server entry point
cmd/worker/           Worker entry point
internal/
  domain/             Core types: Task, TaskStatus (status.go), TaskType, Payload (payload.go)
  application/        Business logic: TaskService
  queue/              Queue interface (abstracts PostgreSQL / future Redis)
  storage/postgres/   PostgreSQL implementation + migrations/
  worker/             Worker pool, mock handlers/
  api/                HTTP router, handlers/
  config/             Environment-based configuration
  observability/      Structured JSON logging
pkg/client/           Go HTTP client for the task API
deployments/          Docker Compose, Dockerfiles
examples/payloads/    JSON payloads for each scenario
```

## Non-Goals

These are explicitly out of scope for this project:

- Real embedding model / vector DB integration (a deterministic, self-contained RAG ingestion exists; no external model API)
- Web UI / Dashboard / Kubernetes
- Exactly-once semantics (the system is at-least-once by design)
- Full Redis backend (only a design skeleton exists)
- Workflow/DAG orchestration (this is a task queue, not Temporal/Conductor)

> Prometheus metrics, task events, manual requeue/cancel, and worker heartbeat
> shipped in Phase 6 and are therefore no longer non-goals.

## Roadmap

- [x] **M1:** PostgreSQL-backed Task API
- [x] **M2:** Worker claim + lease with `FOR UPDATE SKIP LOCKED`
- [x] **M3:** Fixed-interval retry + task timeout
- [x] **M4:** Dead Letter Queue + query
- [x] **M5:** Docker demo + README polish
- [x] **Phase 6:** Task events audit log, manual requeue, cancel, heartbeat, Prometheus metrics, Redis backend skeleton
- [x] **P1:** Idempotent enqueue, fencing token (`lease_version`), atomic state+event transactions, retryable classification, exponential backoff + jitter
- [x] **P2:** Test evidence — concurrent-claim correctness (`go test -race`), worker timeout, e2e lifecycle, per-process test databases
- [x] **P3:** Queue governance (per-queue concurrency caps, pause/resume) + richer metrics (queue depth, oldest-pending age)
- [x] **P4:** Benchmarks (enqueue + full task lifecycle throughput)
- [x] **P5:** Failure Lab (crash → lease expiry → fencing recovery), per-queue rate limiting, Grafana observability pack, lightweight real RAG ingestion, loadgen report

### Future Ideas

- OpenTelemetry distributed tracing
- Full Redis backend implementation (Lua scripts for atomic claim)
- Archive / retention for completed tasks
- `task_events` streaming / WebSocket
- Worker dashboard / Web UI
- Kubernetes deployment manifests

## Testing

The test suite runs against a real PostgreSQL instance. Each test package
provisions its own database (via `internal/testutil`), so packages run safely in
parallel.

```bash
# One-time: start a test Postgres (or set TEST_DATABASE_URL)
docker run -d --name tasklab-test -p 5433:5432 \
  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=tasklab_test postgres:16-alpine

go test ./...            # unit + integration
go test -race ./...      # race detector (concurrent-claim correctness)
go test ./internal/storage/postgres/ -run TestConcurrentClaimNoDuplicates -v
```

Highlights:

- **Concurrent claim**: 8 workers claim 100 tasks with zero duplicate claims.
- **Worker timeout**: a timing-out task still persists its failure (regression test).
- **e2e lifecycle**: pending → running → retry → DLQ → requeue → cancel.

## Performance

Benchmarks measure enqueue and full task-lifecycle throughput:

```bash
go test ./internal/storage/postgres/ -run '^$' -bench . -benchtime=5s
```

Measured on a developer laptop (Dockerized Postgres over a port-mapped connection,
so these are conservative):

| Benchmark | Result |
|-----------|--------|
| `BenchmarkEnqueue` | ~5.7 ms/op (~175 enqueues/s) |
| `BenchmarkTaskLifecycle` | ~19 ms/op (~53 tasks/s end-to-end) |

Each operation is a short transaction that trades raw throughput for correctness
(atomic state + event writes, `SKIP LOCKED` claim, fencing-token ownership). A
co-located/tuned Postgres is significantly faster; the design prioritizes
correctness under concurrency and crashes over throughput.

### Load Test (`make loadgen`)

`cmd/loadgen` runs an in-process load test: N tasks, M worker pools, then reports
throughput, claim-latency percentiles, and duplicate-claim detection. Measured on
a developer laptop (Dockerized Postgres, 4 workers × 4 concurrency):

```text
tasks submitted   : 500
workers           : 4 (concurrency 4 each)
throughput        : 593 tasks/sec
success           : 500
dlq               : 0
claim latency p50 : 90ms
claim latency p95 : 102ms
claim latency p99 : 102ms
duplicate claims  : 0
```

Run with `LOADGEN_TASKS=1000 LOADGEN_WORKERS=8 make loadgen` to reproduce.

## Development

```bash
make build    # Build binaries
make api      # Run API server locally
make worker   # Run worker locally
make test     # Run all tests
go test -race ./...   # Run tests with race detector
make fmt      # Format code
make logs     # View Docker logs
```

## License

MIT
