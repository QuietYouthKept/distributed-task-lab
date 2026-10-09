# Wave 2 implementation report

Date: 2026-10-09 (Asia/Shanghai). Scope is the implementation actually present in the Phase 1 worktree; this report does not certify production readiness.

## Implemented in this continuation

- `internal/artifacts/{store,local,s3,keys}.go`: shared Store contract, bounded local/S3 objects, conditional create, integrity metadata/hash checks and typed error classes. `ObjectInfo.Key` is the logical key; bucket/prefix remain backend configuration and do not leak into references.
- `internal/worker/handlers/rag_ingestion_real.go`: input/output Store injection was already present; when the worker is explicitly configured with the S3 backend, the shared Store now supplies both input and output objects.
- `internal/worker/handlers/llm_eval.go`: deterministic shard outputs use Store `Get`/`PutIfAbsent`; reuses are checked against the immutable run/shard identity and return an `artifact://` logical-key reference.
- `internal/worker/handlers/llm_gateway_eval.go`: per-case provider receipts are immutable Store objects, allowing a different worker instance to resume; final JSONL, manifest, summary and failures are conditionally persisted through Store. This remains a gateway integration boundary, not evidence of a real external provider.
- `internal/worker/handlers/strategy_replay.go`: RAG replay's raw report is stored through Store and emits a logical `artifact://` reference.
- `internal/worker/handlers/composite.go`, `cmd/worker/main.go`: deterministic Eval, Gateway Eval, RAG and Replay receive a common Store when S3 is explicitly selected. Startup verifies the bucket. Production admission remains fail-closed.
- Tests cover Store logical keys with prefixes, shared Store reuse across handler instances, Gateway per-case receipt recovery, and rejection of an Eval object whose bytes do not match its immutable execution identity.
- The only fix for the later PostgreSQL Race failure was test-only: `claimOne` now has a bounded 10-second polling window and queries `clock_timestamp()` plus the earliest pending `next_run_at`; production scheduling code was not changed.
- `scripts/dev/check-env.ps1`: Windows PowerShell preflight reports dependency status without printing credential values.
- Follow-up CI/S3 hardening: `.github/workflows/ci.yml` now has a required MinIO contract job pinned to the locally verified image digest; credentials are generated per CI run and are never committed. `internal/artifacts/minio_integration_test.go` no longer falls back to `minioadmin`, and now checks concurrent conflicting writers, missing objects, cancellation, invalid credentials and concurrent identical writers.
- Stable API contract hardening: `api/openapi.json` now declares the required reader/operator/admin role per `/v1` operation, Bearer authentication, 401/403/429 error responses and `Retry-After` for throttling. `TestOpenAPIV1RolesAuthErrorsAndRouteSet` checks exact path/method coverage, unique operation IDs, auth scheme, least required roles and common error envelopes; CI runs this gate explicitly.

## Not implemented or not verified

- Side Effect still uses a local simulated provider receipt. It is not a real payment/notification provider and is deliberately not promoted to production.
- Worker production startup is still rejected by `Config.ValidateWorker`; no production mode has been unlocked.
- No dual-container API + two-worker E2E, worker crash-after-object-put scenario, or MinIO response-loss proxy was executed in this continuation. The MinIO adapter contract did pass under Race against a fresh temporary container from the pinned local image digest with random one-run credentials. The container used no persistent volume and was auto-removed. This proves the adapter contract only; it is not dual-worker E2E or an AWS/production S3 claim. GitHub Actions has not been remotely run.
- No database least-privilege runtime role, external IdP, provider sandbox, 100k-page benchmark, remote CI run, OTLP receiver test, or backup/restore rehearsal was added here.
- No schema migration was added in this continuation; migrations 010 and 011 from the existing worktree remain in scope and were exercised by PostgreSQL tests.

## Compatibility, rollback, and correctness

Handler result fields that identify persisted artifacts now use `artifact://<logical-key>` rather than worker-local absolute paths. Consumers must resolve those references via the configured Store, not `os.ReadFile`. The Store prefix is configured out of band. Existing LocalStore remains available for development and tests.

The new handler work has no database schema dependency; rollback is a code revert to the prior implementation checkpoint. Existing identity/audit and keyset migrations are separate pre-existing work and must not be removed as part of this rollback. Conditional writes prevent silent replacement; stale Worker fencing and task ACK remain PostgreSQL responsibilities. This is at-least-once execution, not exactly-once execution.
