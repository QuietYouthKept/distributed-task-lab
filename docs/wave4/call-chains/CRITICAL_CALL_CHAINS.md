# Critical execution call chains

These call chains follow the current source paths; exact line numbers intentionally are not frozen so links survive small refactors. The focused tests listed after each chain are the executable evidence, when applicable.

## 1. API task submission and idempotency

`cmd/api/main.go` creates PostgreSQL queue → `api.NewRouterWithConfig` applies request safety, auth and role checks → `handlers.TaskHandler.Create` decodes the stable task request and applies identity/ownership → `application.TaskService.Enqueue` validates type/payload/admission → `PostgresQueue.Enqueue` inserts queue/task and submitted event transactionally → response returns canonical task state.

Invariants: payload is validated before persistence; principal ownership comes from verified identity, not caller-controlled fields; idempotency reuse must match the same principal/key and request fingerprint; the task and submitted event commit together. Tests: `internal/api` OpenAPI/router contract tests, `internal/application` service tests, and PostgreSQL identity/idempotency integration tests.

## 2. Worker claim and queue admission

`cmd/worker/main.go` builds `worker.Pool` → `claimLoop` computes bounded local capacity → `ClaimTasks` locks eligible queue authorities, applies pause/rate/concurrency/aging, selects due pending or expired-running work using PostgreSQL and `FOR UPDATE SKIP LOCKED` → claim transaction sets owner, lease expiry, attempt/generation and event → executor receives the leased task.

Invariants: local prefetch is bounded; row selection and lease update are atomic; database time controls due/expiry decisions; claims on a contended queue should not prevent independent queues from progressing; each new owner generation advances the fencing token. Tests: Postgres queue-governance, `SKIP LOCKED`, fairness/aging, contention, reclaim and Race tests.

## 3. Lease renewal and execution context

`Pool.executeLoop` receives a task job → handler context includes trusted task ID, attempt and generation metadata → renewal goroutine periodically calls `RenewLease` with worker ID and lease version → heartbeat advertises process health → failed renewal cancels the handler context; a late completion still must pass repository fencing.

The handler can outlive lease ownership if it ignores cancellation; the system cannot forcibly stop a Go goroutine. Lease expiry permits duplicate execution, while fencing protects durable task state. Tests: `internal/worker/pool_test.go`, lease renewal and stale-generation PostgreSQL integration tests.

## 4. Successful completion

Handler result → `MarkSuccess` starts a transaction → conditional update requires `status=running`, matching `leased_by`, and current `lease_version` → success event inserted → commit → Worker releases local slot. Zero updated rows is a fencing rejection, not success.

Invariant: stale Worker cannot overwrite a newer generation's state. Test: stale-owner completion integration tests and Race suite.

## 5. Retry/backoff and DLQ

Handler failure → Worker classifies retryability → `MarkFailure` locks current task generation → PostgreSQL `clock_timestamp()` becomes the scheduling reference → retry delay is calculated from attempt/base/max/multiplier/jitter → row becomes pending with `next_run_at`; event is inserted in the same transaction → after commit `tasklab_retry_total` increments. Non-retryable or exhausted attempts go to DLQ.

Claim must reject a retry before `next_run_at`; after expiry it can eventually claim. Jitter is intentionally not a precise wall-clock assertion. Tests: retry backoff Postgres test, attempt/DLQ tests, and concurrent claimant Race cases.

## 6. Lease-expiry recovery

Worker process loss → lease remains until its database expiry → another claim transaction marks an exhausted attempt DLQ or reclaims eligible work → generation advances → duplicate handler execution is possible → only the current generation can commit. Test: API/two-Worker crash recovery and Postgres reclaim tests.

## 7. Cursor/keyset pagination

`TaskHandler.List` normalizes filter + owner scope → HMAC cursor parser verifies format, query binding, key ID/TTL and signature → repository orders on `(created_at DESC, id DESC)` and filters on tuple `< (cursor.created_at, cursor.id)` → next cursor encodes the last returned key. Cursor-v2 rotation is an opt-in candidate; v1 remains the default. Test: API cursor contract, tamper/filter binding and Postgres pagination benchmark.

## 8. Local or S3 artifact access

Worker handler validates typed task payload → resolves an allow-listed artifact reference → `artifacts.Store` chooses local or S3 implementation from configuration → put-if-absent/content metadata checks preserve immutable keys and integrity → failure is returned to task policy; metrics count only unexpected backend errors and omit keys/errors from labels. Tests: artifact package unit tests, MinIO contract and two-Worker shared-artifact recovery.

Local storage is not a cross-host shared backend. Production uses S3 only when its endpoint, TLS, credentials and bucket policy pass production validation; no actual cloud credentials were used.

## 9. External-effect ledger and reconciliation

Handler computes stable operation/effect identity → `RecordIntent` persists request hash, policy and ownership generation → provider boundary is called with idempotency key → returned receipt is recorded, or an uncertain network outcome becomes `unknown` → reconciliation queries the provider contract or requires an audited manual resolution → ledger does not itself complete task state.

The effect ledger avoids blind replay; it cannot infer a provider outcome without provider-specific lookup support. Tests: response-loss, stable identity, immutable receipt and manual-resolution Race integration tests.

## 10. Shared ingress rate-limit decision

Router authenticates principal → optional PostgreSQL limiter hashes identity → bounded slot acquisition/update uses row lock and database statement timestamp → decision returns allowed or retry-after → shared mode remains off by default; process-local limiter remains an overload guard. Test: two separately built API containers (16 admitted / 48 rate-limited at burst 16), strict Postgres integration and API Race.

## 11. Metrics and alerts

API/Worker runtime metrics are process-local and scraped separately → Prometheus evaluates `deployments/observability/tasklab-alerts.yml` → local E2E stopped/restored an isolated API and observed Normal/Firing/Resolved. This verifies rule evaluation and target recovery only; there is no Alertmanager/on-call delivery evidence.

## 12. Shutdown and migration startup

SIGTERM → API stops accepting new requests and drains with `http.Server.Shutdown`; Worker stops claims, allows owned handlers to drain while leases renew, then cancels on deadline → process exits. On startup, API and Worker validate config, connect to PostgreSQL and serialize forward-only migrations using an advisory lock. The release smoke verified Worker drain, not production rolling upgrade. Newer-schema/older-binary rollback is rejected by migration/readiness guards and remains blocked pending compatible deployment strategy.
