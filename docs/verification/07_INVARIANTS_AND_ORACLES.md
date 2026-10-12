# Distributed Invariants and Test Oracles

These are source-level contracts and concrete oracles; this page is not a claim that the current SHA has passed PostgreSQL acceptance.

| Invariant | Implementation evidence | Oracle / test evidence | Unclosed risk |
|---|---|---|---|
| A task has one valid status among pending/running/success/failed/dlq; persisted status is constrained in PostgreSQL. | `internal/domain/status.go`; `internal/storage/postgres/migrations/007_integrity_and_runtime.sql` | Domain validation and DB constraint tests; historical PostgreSQL evidence only | Current SHA integration not run. |
| A due pending task is claimable only when PostgreSQL `clock_timestamp()` reaches `next_run_at`; lease/retry scheduling uses DB time, not caller wall-clock authority. | `internal/storage/postgres/task_repository.go:ClaimTasks`, `MarkFailure`, claim/reclaim SQL | `TestLeaseUsesDatabaseClockAuthority`, retry-backoff tests | Current SHA tests not run; retry remains bounded only when policy/attempt limits are valid. |
| A claim/reclaim establishes a new lease generation; terminal/renewal writes match task ID, worker ID, running state and lease version. | `task_repository.go:ClaimTasks`, `MarkSuccess`, `MarkFailure`, `RenewLease`; migration 004 | Stale generation integration tests and Failure Lab oracle | Table-level Worker UPDATE permission lets a credential holder bypass application predicates through direct SQL. |
| Enqueue idempotency is represented by a unique key and converges same-key calls to one logical row. | Migration 004 unique index; `task_repository.go:Enqueue` | `TestConcurrentEnqueueIdempotency`, owner isolation tests | Client timeout after DB commit and same-key/different-payload across independent processes require current DB/API proof. |
| State transition and event insert are committed atomically through supported repository operations. | Transactional repository methods in `task_repository.go` | Audit/effect integration tests and historical DB run | Current source transaction-level failure injection not rerun. |
| External execution is at-least-once; ledger/reconciliation can make a business effect idempotent only when the provider contract supports it. No exactly-once provider execution is claimed. | `internal/storage/postgres/effect_ledger.go`, `internal/effects`, Worker handlers | Effect response-loss/reconciliation tests | Provider-specific idempotency and reconciliation remain unverified absent a real provider. |
| Production execution remains fail-closed; local staging simulation does not widen production allowlist. | `internal/config`, `internal/domain/taskpayload`, worker validation | negative config/payload tests, historical run | Re-validate after every future execution-policy change. |

Current versus historical status is detailed in `03_HISTORICAL_EVIDENCE_AUDIT.md`; this table is a source map and oracle catalog.
