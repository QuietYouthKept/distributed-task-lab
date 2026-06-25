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

## Architecture

```
Client / Demo Script
        │
        ▼
   API Server          ───   POST /tasks
        │                    GET  /tasks/{id}
        │                    GET  /tasks
        ▼
   PostgreSQL           ───   Task state center
        ▲
        │
  ┌─────┴─────┐
  │           │
Worker 1   Worker 2   ───   Independent goroutine pools
```

### Key Mechanisms

| Mechanism | Description |
|-----------|-------------|
| **Lease** | Workers claim tasks in a DB transaction with `FOR UPDATE SKIP LOCKED`. Each claim sets `leased_by` and `lease_expires_at`. If a worker crashes, the lease expires and another worker picks up the task. |
| **Retry** | Failed tasks increment `attempt` and are re-scheduled with `next_run_at = now + retry_delay`. Fixed-interval retry in MVP. |
| **Timeout** | Each task has a `timeout_seconds`. If execution exceeds it, the task is marked as failed and retried. |
| **DLQ** | When `attempt >= max_attempts`, the task moves to DLQ (`status = 'dlq'`) with the failure reason recorded. |

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) & Docker Compose
- [Go](https://go.dev/dl/) 1.22+ (for local development)
- `curl` and `jq` (for the demo script)

### One-Command Start

```bash
# Start everything: PostgreSQL + API + 2 Workers
make up

# Or manually:
docker compose -f deployments/docker-compose.yml up -d
```

### Run the Demo

```bash
# Full workflow: success → retry → DLQ
bash scripts/demo.sh

# Or individual scenarios:
make demo-success     # Task completes successfully
make demo-retry       # Task fails, retries
make demo-dlq         # Task exhausts retries → DLQ
make demo-timeout     # Task times out
```

### Quick API Test

```bash
# Submit a RAG ingestion task
curl -X POST http://localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "rag_ingestion",
    "payload": {
      "version": "1",
      "kind": "rag_ingestion",
      "data": {
        "document_id": "doc-001",
        "dataset_id": "ds-demo",
        "index_name": "demo-index"
      }
    }
  }'

# Check status (use the returned ID)
curl http://localhost:8080/tasks/<task-id>

# List all tasks
curl http://localhost:8080/tasks

# List DLQ tasks
curl "http://localhost:8080/tasks?status=dlq"
```

## Task States

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

## Task Types

| Type | Description | MVP Status |
|------|-------------|------------|
| `rag_ingestion` | Mock RAG document ingestion (chunking, embedding) | ✅ Implemented |
| `eval` | Mock model evaluation | ✅ Stub |
| `benchmark` | Mock benchmark run | ✅ Stub |

All handlers currently use mock implementations. The architecture supports
replacing them with real handlers later.

## Project Structure

```
cmd/api/              API server entry point
cmd/worker/           Worker entry point
internal/
  domain/             Core domain types (Task, TaskStatus, TaskType)
  application/        Business logic (TaskService)
  queue/              Queue interface (abstracts PostgreSQL / future Redis)
  storage/postgres/   PostgreSQL implementation + migrations
  worker/             Worker pool, runner, task handlers
  api/                HTTP router and handlers
  config/             Environment-based configuration
  observability/      Structured logging, metrics placeholders
pkg/client/           Go HTTP client for the task API
deployments/          Docker Compose, Dockerfiles
examples/payloads/    Example JSON payloads for each task type
scripts/              Demo script
```

## At-Least-Once Semantics

This system guarantees **at-least-once** delivery: every task will be attempted
at least once, but may be executed more than once if:

- A worker crashes after starting execution but before marking success/failure.
- A lease expires and another worker claims the same task.
- Network issues cause retry timeouts.

**Task handlers must be idempotent.** For example, a RAG ingestion handler
should check `document_id + version` and skip re-ingestion if the document
is already indexed.

## Non-Goals (MVP)

These are explicitly out of scope for v1 but noted for clarity:

- Real RAG ingestion / AI model API calls
- Web UI / Dashboard
- Kubernetes deployment
- Exactly-once semantics
- Prometheus `/metrics` endpoint
- Manual requeue from DLQ (`POST /tasks/{id}/requeue`)
- Cancel API
- Worker heartbeat / stale worker detection
- Redis backend
- `task_events` audit table

## Roadmap

- [x] **M1:** PostgreSQL-backed Task API (`POST /tasks`, `GET /tasks/{id}`, `GET /tasks`)
- [x] **M2:** Worker claim + lease with transaction-correct locking
- [x] **M3:** Fixed-interval retry + task timeout
- [x] **M4:** Dead Letter Queue (DLQ) + query
- [x] **M5:** Docker Compose + Makefile + demo script

### Phase 2 Ideas

- Exponential backoff with jitter for retries
- Worker heartbeat and stale worker detection
- Redis backend (same `Queue` interface)
- `task_events` table for audit trail
- Manual DLQ requeue API
- Prometheus metrics
- Graceful shutdown improvements (drain in-flight tasks)

## Development

```bash
# Run locally (needs PostgreSQL running)
make api      # Start API server on :8080
make worker   # Start a worker

# Build
make build

# Test
make test
```

## License

MIT
