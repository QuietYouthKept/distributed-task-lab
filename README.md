# distributed-task-lab

> A correctness-first PostgreSQL task queue laboratory in Go.

`distributed-task-lab` is a small execution kernel for studying and testing the
hard parts of distributed background work: `SKIP LOCKED`, leases, fencing
tokens, crash recovery, strict per-queue governance, shared rate admission,
failure injection, and reproducible correctness evidence.

PostgreSQL is the durable authority. Workers are disposable. Execution is
**at least once**; external business effects require idempotency. This project
does not claim end-to-end exactly-once semantics.

## Guarantees at a glance

| Property | Mechanism | Automated evidence | Guarantee | Known limitation |
|---|---|---|---|---|
| No concurrent duplicate claim generation | task row `FOR UPDATE SKIP LOCKED` | `TestConcurrentClaimNoDuplicates` | one committed owner generation per task row | a crashed handler may execute again after expiry |
| Strict queue concurrency | queue authority row locked before capacity decision | `TestQueueMaxConcurrencyConcurrent` | committed `running(queue) <= max_concurrency` for pending admission | `0` means unlimited |
| Independent queue admission | eligible queue authorities acquired with `FOR UPDATE SKIP LOCKED` before task rows | `TestClaimSkipsContendedQueueAuthority` | contention on queue A does not head-of-line block ready queue B | a database-wide outage still affects every queue |
| Expired-running recovery | reclaim replaces, rather than adds, a running slot | `TestExpiredRunningReclaimIgnoresCapacity` | expired work can recover even when the queue is at capacity | duplicate execution remains possible |
| Stale DB writes fenced | `(task_id, worker_id, lease_version)` predicates | stale success/failure/renew tests | stale generations cannot ack or renew | fencing cannot undo an external side effect |
| Long handler ownership | renewal starts at claim time and runs every lease/3 | `TestPoolRenewsLeaseDuringLongHandler` | a healthy worker keeps a live generation leased | renewal failure cancels cooperatively; handlers must honor context |
| Queue rate | token state serialized in the PostgreSQL queue row | `TestGlobalQueueRateLimitAcrossWorkersAndStableRefresh` | cluster-combined claim admission follows rate/burst | admission rate is not downstream API completion rate |
| Enqueue idempotency | unique `idempotency_key` plus atomic submitted event | `TestConcurrentEnqueueIdempotency` | callers converge on one task row | not handler exactly-once |
| Pause | queue authority checked in claim transaction | `TestQueuePausePreventsClaim` | no new claims while paused | already-running tasks continue |
| Graceful drain | stop claims, keep renewals, drain, deadline cancel | worker pool tests and failure lab | cooperative in-flight handlers can ack before exit | `SIGKILL` skips drain; lease recovery applies |

See [guarantees](docs/guarantees.md), [failure model](docs/failure-model.md), and
[architecture](docs/architecture.md) for precise scope.

## Architecture

```text
HTTP clients -> API -> PostgreSQL tasks / queues / task_events
                              ^
                              |
                 disposable worker processes
                 bounded claim -> renew -> handle -> fenced ack
```

Queue rows are serialization authorities for aggregate admission. Task rows are
ownership authorities. Queue locks are acquired in sorted name order when a
claim touches multiple queues. Database `clock_timestamp()` decides scheduling,
lease expiry, renewal, retry base time, and heartbeat age.

## Quick start

```bash
export API_TOKEN='replace-with-a-development-token'
export ADMIN_TOKEN='replace-with-a-distinct-admin-token'
make up
curl -X POST http://localhost:8080/tasks \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $API_TOKEN" \
  -d '{"type":"rag_ingestion","payload":{"version":"1","kind":"rag_ingestion","data":{"document_id":"doc-1"}}}'
```

The bundled Compose deployment fails closed unless distinct `API_TOKEN` and
`ADMIN_TOKEN` values are supplied. Direct binary execution permits empty tokens
only for deliberate local development. `/live` checks the process and `/ready`
checks PostgreSQL plus the expected schema version. Worker health is on port
`9091` by default.

## Verification

```bash
make test          # production packages; PostgreSQL tests may skip locally
make integration   # REQUIRE_TEST_DATABASE=1: unavailable DB is a failure
make race
make vet
make bench         # PostgreSQL on localhost:5433
make demo-failure-lab
make loadgen
make benchmark-matrix
ASSETLAB_RESULT_DIR=/new/evidence/path DATABASE_URL=... go run ./cmd/assetlab
```

The repository may contain local `Round*` study snapshots. Make/CI intentionally
target `cmd`, `internal`, and `pkg`, which are the production module surface.
Asset-derived, executable cards are in [`test-cards`](test-cards); the reusable
PostgreSQL history/oracle runner is [`cmd/assetlab`](cmd/assetlab), and real
container fault orchestration is [`scripts/run_asset_docker_faults.ps1`](scripts/run_asset_docker_faults.ps1).

## Core configuration

| Variable | Default | Purpose |
|---|---:|---|
| `DATABASE_URL` | local tasklab PostgreSQL | durable authority |
| `WORKER_ID` | hostname + pid + boot UUID | explicit override for unique process identity |
| `WORKER_CONCURRENCY` | `4` | active handlers |
| `CLAIM_BATCH_SIZE` | `5` | upper claim batch; local ownership remains bounded |
| `LEASE_TIMEOUT` | `90s` | ownership lease; renewal interval is lease/3 |
| `SHUTDOWN_TIMEOUT` | `30s` | graceful drain deadline |
| `DB_OPERATION_TIMEOUT` | `10s` | critical SQL deadline |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | `25` / `5` | connection pool bounds |
| `API_TOKEN` | empty in binary; required by Compose | general bearer token |
| `ADMIN_TOKEN` | empty in binary; required by Compose | queue/DLQ/cancel bearer token |
| `MAX_REQUEST_BODY_BYTES` | `1048576` | HTTP body limit |
| `RAG_ARTIFACT_DIR` | `artifacts/index` | durable mount for RAG, eval, and simulated receipts |
| `EVAL_BACKEND` | `deterministic` | `deterministic` for CI or `gateway` for a real evaluator |
| `EVAL_GATEWAY_URL` / `EVAL_GATEWAY_PROFILE` | unset | required only for the Gateway backend |
| `EVAL_GATEWAY_API_KEY` | unset | secret bearer token; never place it in task data |
| `EVAL_REQUEST_TIMEOUT` / `EVAL_PROVIDER_CONCURRENCY` | `30s` / `8` | per-call timeout and in-process gateway cap |

## StrategyOps RAG replay

`strategy_replay` supports the existing `new_user_feed_exploration` payload and the
`rag_retrieval_policy` payload used by StrategyOps. A RAG replay requires at least 100 cases,
strict `top_k`/`reranker_enabled`/`min_score` configuration, and immutable
`dataset_version`, `prompt_version`, `retriever_version`, `producer_commit`, and
`evaluator_schema_version` fields.

The worker writes a content-addressed JSON report under
`$RAG_ARTIFACT_DIR/rag-replay/` and returns its URI and SHA-256 together with Recall@k, MRR,
nDCG, answer-point recall, unsupported-claim rate, refusal accuracy, latency, token, and cost
metrics. Safety, latency, or cost breaches are successful `REGRESSION` results—not worker errors—
so StrategyOps can fail approval closed.

The bundled scorer is deterministic development scaffolding. It preserves the production-facing
receipt, provenance, hash, retry, and reconciliation contract, but does not claim to evaluate an
external enterprise retriever until such an evaluator is wired into this handler.

## Semantics that matter

- A claim increments both `attempt` and `lease_version`.
- An expired-running reclaim remains `running`; it replaces ownership and does
  not consume another max-concurrency slot.
- `MarkSuccess`, `MarkFailure`, and `RenewLease` require task ID, worker ID, and
  lease version. Renewal also refuses an already-expired generation.
- Shared queue token buckets are consumed under the queue row lock. Refreshing
  unchanged config preserves token history.
- A handler may perform an effect and crash before ack. Retrying can repeat that
  effect. Use a stable business idempotency key at the effect boundary.
- `task_events` is atomic transition history, not an immutable compliance audit
  log; it currently follows task retention via `ON DELETE CASCADE`.
- Scheduling is strict priority by default. Queues can opt into an audited
  aging boost; see [priority fairness](docs/fairness.md).

## Documentation

- [Architecture](docs/architecture.md)
- [Guarantees](docs/guarantees.md)
- [Failure model](docs/failure-model.md)
- [Failure lab](docs/failure-lab.md)
- [Migrations](docs/migrations.md)
- [Handler idempotency](docs/handler-idempotency.md)
- [Operations](docs/operations.md)
- [Benchmark methodology](docs/benchmark-methodology.md)
- [LLM eval shards](docs/llm-eval.md)
- [External side-effect adapter](docs/side-effect-adapter.md)
- [Priority fairness](docs/fairness.md)
- [Database capacity analysis](docs/database-capacity.md)
- [Storage lifecycle](docs/storage-lifecycle.md)
- [Review bundle](docs/review/FINAL-REVIEW-GUIDE.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

## Non-goals

This repository does not aim to replace Temporal, River, or Graphile Worker. It
does not provide a workflow DSL, web UI, scheduler catalog, Kafka/Redis control
plane, or transactional exactly-once external effects. The scope is a compact,
auditable PostgreSQL execution kernel.
