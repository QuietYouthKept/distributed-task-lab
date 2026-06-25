# Roadmap

## Milestone 1 — PostgreSQL-backed Task API ✅
- POST /tasks, GET /tasks/{id}, GET /tasks
- PostgreSQL tasks table with migrations
- Docker Compose: API + PostgreSQL

## Milestone 2 — Worker Claim + Lease ✅
- Worker process with goroutine pool
- Transaction-correct ClaimTasks with FOR UPDATE SKIP LOCKED
- Lease mechanism (leased_by, lease_expires_at)
- Mock RAG ingestion handler

## Milestone 3 — Retry + Timeout ✅
- Fixed-interval retry (attempt, max_attempts, retry_delay_seconds)
- Task timeout with context cancellation
- last_error recording

## Milestone 4 — DLQ + Query ✅
- Dead Letter Queue (status = dlq)
- DLQ reason recording
- GET /tasks?status=dlq query

## Milestone 5 — Docker + Makefile + README ✅
- docker-compose.yml with PostgreSQL + API + Worker
- Dockerfile.api, Dockerfile.worker (multi-stage)
- Makefile with up/down/build/demo targets
- Demo script (scripts/demo.sh)
- README.md with architecture, quick start, non-goals
- DESIGN.md with design decisions

## Phase 2 (Future)
- Exponential backoff with jitter
- Worker heartbeat / stale worker detection
- Redis backend (same Queue interface)
- task_events audit table
- Manual DLQ requeue (POST /tasks/{id}/requeue)
- Prometheus metrics
- Real RAG ingestion with vector DB integration
