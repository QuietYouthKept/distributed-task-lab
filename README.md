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

# One command — the full story (success → retry → DLQ):
make demo

# Or step by step:
make demo-claim    # Multi-worker claiming
make demo-retry    # Automatic retry on failure
make demo-dlq      # Dead Letter Queue after max retries
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
| `GET` | `/tasks?status=dlq` | List DLQ tasks |
| `GET` | `/dlq` | Same as above (convenience) |
| `GET` | `/stats` | Task counts by status |
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

## Retry, Timeout, and DLQ

### Retry (Fixed-Interval)

- On failure: `status → pending`, `next_run_at = now + retry_delay_seconds`, lease cleared
- `attempt` is incremented on each claim (in `ClaimTasks`), not on each failure
- When `attempt < max_attempts`: task is retried
- When `attempt >= max_attempts`: task enters DLQ

### Task Timeout vs Lease Timeout

| Concept | Meaning | Example |
|---------|---------|---------|
| `timeout_seconds` | Max time a handler may run | 60s |
| `lease_timeout` | How long a worker owns a task | 90s |

**Rule:** `lease_timeout >= timeout_seconds + buffer` — without heartbeat,
the buffer prevents premature re-claiming.

### DLQ (Dead Letter Queue)

- Tasks that exhaust all retry attempts enter `status = 'dlq'`
- `dlq_reason` records "max attempts exceeded: <last error>"
- DLQ tasks are **never claimed** by workers (excluded by the WHERE clause)
- Query via `GET /tasks?status=dlq` or `GET /dlq`
- Manual requeue is a planned Phase 2 feature

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

## Non-Goals (MVP)

These are explicitly out of scope for v1:

- Real RAG ingestion / AI model API calls
- Web UI / Dashboard / Kubernetes
- Exactly-once semantics
- Prometheus `/metrics` endpoint
- Manual requeue / cancel API
- Worker heartbeat / stale worker detection
- Redis backend
- `task_events` audit table

## Roadmap

- [x] **M1:** PostgreSQL-backed Task API
- [x] **M2:** Worker claim + lease with `FOR UPDATE SKIP LOCKED`
- [x] **M3:** Fixed-interval retry + task timeout
- [x] **M4:** Dead Letter Queue + query
- [x] **M5:** Docker demo + README polish

### Future Ideas

- Exponential backoff with jitter
- Worker heartbeat and stale worker detection
- Redis backend (same `Queue` interface)
- `task_events` audit table
- Manual DLQ requeue
- Prometheus metrics

## Development

```bash
make build    # Build binaries
make api      # Run API server locally
make worker   # Run worker locally
make test     # Run all tests
make fmt      # Format code
make logs     # View Docker logs
```

## License

MIT
