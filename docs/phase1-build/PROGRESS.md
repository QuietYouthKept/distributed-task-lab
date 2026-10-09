# Phase 1 build progress

Updated: 2026-10-09 (Asia/Shanghai)

## Scope and recovery point

- Target: `codex/phase1-production-hardening`; this is the explicit Phase 1 checkout, not the separately open `codex/real-ai-chaos` checkout.
- Starting revision for this batch: `09e9deaba91e6941bc4c7c7cc00f37c4a4be8dea` (audit-only documentation commit). Phase 0 tag remains unchanged.
- The user authorized continuous implementation across packages A–D, local commits, migrations and infrastructure. No push/release tag. Never modify historical evidence or logs.
- Historical `evidence/index.json` still identifies Phase 0 commit `f8df921`; it does not validate current Phase 1 changes.
- Do not mark an item DONE without its tests and evidence; a Fake Provider or skipped integration is not real provider/MinIO E2E.

## Engineering package A — production admission, identity, RBAC, audit, security

- Status: IMPLEMENTED LOCALLY; full repository unit and race suites pass.
- Implemented files: `internal/auth/*`; `internal/config/config.go`; `internal/api/router.go`; `internal/api/handlers/task_handler.go`; `internal/application/task_service.go`; `internal/domain/task.go`; `internal/domain/taskpayload/registry.go`; `internal/storage/postgres/migrations/010_identity_audit.sql`; `internal/storage/postgres/task_repository.go`; `cmd/api/main.go`; `cmd/worker/main.go`; `internal/worker/handlers/composite.go`; `docs/adr/ADR-0001-identity-resource-scope-and-audit.md`.
- Tests/results: targeted auth/API/config/application/worker tests passed; required-PostgreSQL integration suite passed. Coverage includes ephemeral RSA/JWKS validation and rotation, role denial, owner isolation, creator-scoped idempotency, atomic audit and fail-closed audit insertion. CI's existing unit/PostgreSQL/race jobs include these paths.
- Current issues/blockers: no external OIDC staging issuance/rotation test; production DB least-privilege runtime role is not proven.
- Next: finish required full unit/race verification; keep production execution allowlist empty.
- Current Git commit: `09e9deaba91e6941bc4c7c7cc00f37c4a4be8dea` (uncommitted implementation follows).

## Engineering package B — shared ArtifactStore and cancellation

- Status: PARTIAL; production remains intentionally fail-closed.
- Implemented files: `internal/artifacts/{store,keys,local,s3}.go`; local Store tests and opt-in MinIO contract test; `internal/worker/handlers/rag_ingestion_real.go` accepts injected source/destination Stores; `internal/config/config.go` has S3 config and production validation; ADR-0002 records remaining gate.
- Tests/results: `go test ./internal/artifacts ./internal/worker/handlers -count=1` passed local-store and handler tests. The MinIO integration test was not run because the container image could not be pulled: Docker Hub returned pull access denied and Quay returned 401. This is not MinIO adapter verification.
- Current issues/blockers: RAG is the only migrated core handler. Eval, gateway eval, strategy replay and side-effect receipt paths still use local files; no two-worker shared-store E2E and no MinIO evidence.
- Next: obtain an accessible pinned MinIO image/registry, run the contract suite, migrate remaining artifact handlers, and prove separate worker processes see shared artifacts before lifting the production worker gate.
- Current Git commit: `09e9deaba91e6941bc4c7c7cc00f37c4a4be8dea`.

## Engineering package C — pagination, API admission, client and OpenAPI

- Status: PARTIAL; signed keyset pagination, client hardening and per-process ingress limiter implemented locally.
- Implemented files: `internal/api/handlers/cursor.go`; `internal/api/handlers/task_handler.go`; `internal/domain/task.go`; `internal/storage/postgres/task_repository.go`; `internal/api/router.go`; `pkg/client/client.go`; `api/openapi.json`; ADR-0003. `/v1/tasks` uses `(created_at DESC, id DESC)` and an HMAC cursor bound to filters, page size and verified subject; max page size is 100. Legacy `/tasks` retains its array contract. Router includes a bounded principal-keyed in-process token bucket with 429/Retry-After. Production requires a configured cursor key of at least 32 bytes.
- Tests/results: cursor tamper/binding, handler pagination, limiter identity scope, client auth/idempotency/context/error/page decoding, OpenAPI route/parameter contract and Postgres keyset tests added. Targeted API/client tests passed; required-Postgres keyset test passed.
- Current issues/blockers: cursor key rotation invalidates outstanding tokens; OpenAPI does not yet have a generated full schema drift gate; the limiter is process-local, not cluster-wide; no API load test.
- Next: strengthen operation/request/response OpenAPI drift checks and add API multi-process load verification if a shared quota becomes a requirement.
- Current Git commit: `09e9deaba91e6941bc4c7c7cc00f37c4a4be8dea`.

## Engineering package D — reconciliation, telemetry, test gates and operations

- Status: PARTIAL; operator effect review and manual resolution endpoints implemented; broader telemetry and operations gates remain.
- Implemented files: `internal/effects/ledger.go`; `internal/storage/postgres/effect_ledger.go`; `internal/api/handlers/effect_handler.go`; `internal/api/router.go`; `cmd/api/main.go`; `api/openapi.json`; `docs/operations.md`; ADR-0004. Listing exposes only unresolved intent/unknown effects. Manual resolution requires authenticated actor, request ID and bounded reason; state update and immutable security audit are one transaction. Effect endpoints always require an auth boundary; resolution requires admin.
- Tests/results: required-Postgres race test covers unresolved listing, principal requirement, actor/request audit, state transition and rollback if the audit insert fails. Existing CI already has unit, PostgreSQL integration and race jobs; full repository unit and race suites pass locally.
- Current issues/blockers: no real payment/notification provider credentials or resolution authority. No comprehensive telemetry-redaction verification, backup/restore/retention runbook or MinIO CI gate.
- Next: provider-specific reconciliation requires sandbox authority; complete safe telemetry tests, operations runbooks, backup/restore exercise, MinIO CI integration and Evidence collection.
- Current Git commit: `09e9deaba91e6941bc4c7c7cc00f37c4a4be8dea`.

## Verification ledger

- PASS: `go test -race ./internal/storage/postgres -run '^TestRetryBackoff$' -count=20 -v` (20/20 after bounded helper update). The first all-package integration attempt had one `claimOne` empty result; no production scheduling defect reproduced.
- PASS: `go test -race ./internal/storage/postgres -count=5` with `REQUIRE_TEST_DATABASE=1`.
- PASS: `go test ./cmd/... ./internal/... ./pkg/... -count=1` and `go test -race ./cmd/... ./internal/... ./pkg/... -count=1`, each with PostgreSQL integration required.
- PASS: `go vet ./...`; `gofmt -l cmd internal pkg` returned no files; `git diff --check`.
- PASS: API router/handler/contract tests rerun after tightening effect endpoint authentication.
- PASS: `go run ./cmd/failurelab`: 12/12 scenarios, twice.
- CHAOS: first immutable local run `benchmarks/chaos/20261009T075847Z` reported 29/30 `postgres_short_disconnect` because the harness attempted one immediate claim. Harness now polls with a 3-second bound and PostgreSQL-clock due check. New run `benchmarks/chaos/20261009T075948Z` passed 30/30 in all 6 scenarios. The first evidence directory was not modified.
- NOT VERIFIED: MinIO adapter integration/cross-process storage and MinIO CI gate. Local Docker image fetch failed (Docker Hub denied image; Quay returned 401); adapter test remained skipped. Do not treat LocalStore tests as S3 contract evidence.
- Historical Phase 0 Evidence remains immutable and does not validate this revision.

## Wave 2 continuation — 2026-10-09

- Workspace revalidated at start: branch `codex/phase1-production-hardening`, start HEAD `09e9deaba91e6941bc4c7c7cc00f37c4a4be8dea`; Phase 0 tag still resolves to `e66cf722c51b740243ae6d6126eea7fe361ada04`. The default terminal opened the separate `distributed-task-lab` checkout first; it contained an unrelated untracked `evidence/index.json` and was not modified. All Phase 1 work stayed in this repository.
- Environment: Windows AMD64 / Go 1.26.6; PostgreSQL 16 `tasklab-test-postgres` accepts host TCP 5433; Windows MinIO health 9000 and Docker MinIO health 19000 both returned 200; `tasklab-wave2-minio` and `tasklab-wave2-net` exist. The current process has no MinIO credential variables. Other projects' running Docker containers were left untouched.
- PASS: required PostgreSQL `go test ./cmd/... ./internal/... ./pkg/... -count=1`; full Race equivalent; PostgreSQL Race `-count=5` (98.960 s); RetryBackoff Race `-count=20`; `go vet ./...`; Failure Lab 12/12.
- PASS: new Chaos run `benchmarks/chaos/20261009T092028Z`: six scenarios, 30 trials each, 30/30 per scenario. Earlier 29/30 and 30/30 run directories remain unchanged. These local runs are ignored by Git.
- Code continued: deterministic Eval and RAG Replay outputs now use Store; Gateway Eval uses one immutable Store object per case receipt and conditional immutable output objects; the Worker can inject configured S3 for these Store-backed paths in non-production deployments. `ObjectInfo.Key` now has logical-key semantics, preserving references when a Store prefix is configured. Mismatched Eval content is rejected. Added a PowerShell preflight script and Wave 2 implementation/verification/security/gap docs.
- PASS after handler changes: `go test ./cmd/worker ./internal/artifacts ./internal/worker/handlers -count=1`, its Race counterpart; full unit+required-Postgres and full Race were rerun and passed.
- NOT VERIFIED: authenticated MinIO contract, dual-container/two-process Worker E2E, crash-after-Put recovery, MinIO fault injection, production least privilege, real external provider/IdP, 100k pagination benchmark, OTLP receiver, remote CI, and backup/restore. Health checks do not prove authenticated object I/O. No credentials were printed or guessed.
- Production worker remains fail-closed. Local Store/S3 adapter availability does not automatically authorize production handlers. No Evidence Index baseline was rewritten.
- Local implementation checkpoint created: `69da9562a4617900755f6157460b059f1adae7a5` (`feat: integrate phase one hardening packages`). No push/tag. The Phase 0 tag and older evidence remain unchanged.
- PASS/NOT VERIFIED Evidence run `wave2-20261009T093214275Z-0b73ce734dcd`: collected from clean start commit `0b73ce734dcd1f09db830bb46524c450c7732239`; 7 commands passed (full unit+PostgreSQL, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab, Chaos Matrix, Vet), MinIO contract was NOT VERIFIED due missing process variables. Run is preserved under `evidence/phase1/runs/`; dedicated Phase 1 index status is NOT VERIFIED. Evidence Commit: `85cf764` (child of the tested implementation/collector commit); Phase 0 `evidence/index.json` remains unchanged.
- Added `scripts/evidence/collect-wave2.ps1`. Initial run preserved as-is. A follow-up collector revision will add raw-log hashes and config/dependency fingerprints before a second clean run, rather than editing the first manifest.
- Current next steps: commit collector hardening, then collect a second run from a clean commit. If MinIO credentials remain unavailable, retain an `unverified` evidence status and stop short of claiming full Wave 2/E2E completion or production readiness.
