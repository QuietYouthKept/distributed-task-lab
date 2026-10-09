# Wave 3 progress

Updated: 2026-10-10 (Asia/Shanghai)

## Recovery point

- Branch: `codex/phase1-production-hardening`.
- Latest pushed commit at start of Wave 3 work: `ac46b6bf2411aa68af671897f6ebdb2e0786e323` (CI follow-up fix).
- PR #1 remains OPEN and Draft.
- CI Run `37962333979`: PASS, including actual source-built MinIO image, S3 contract, API/two-Worker recovery, PostgreSQL backup/restore, PostgreSQL integration, full Race, and public Go Client module.
- Security Run `37962334001`: govulncheck, CodeQL, and Secret Scan PASS; Scorecard was conditionally SKIPPED.
- Prior failed runs `37956337334`, `37956336466`, and `37960805439` remain unchanged.
- Wave 2 Evidence `wave2-20261009T153618132Z-a292ec7a996b` stays bound to implementation `a292ec7a996bc39214ab413c420eacd9268a7298`; it does not validate new code.

## Completed in this continuation

- CI follow-up fix was pushed as `ac46b6b`; remote CI and Security results above are bound only to that SHA.
- The MinIO JSON checksum parser now accepts the trailing comma emitted by Go 1.26.9; failures report non-secret diagnostics. Module and license hashes are verified before image construction.
- `test/external-client/go.mod` now targets the root module's Go 1.26.9 toolchain.
- Began Wave 3 package B: added provider-independent HTTP Provider port and contract client with outcomes that distinguish not-sent, confirmed rejection/success, explicit retryable rejection, and unknown. Added HTTP contract/fault tests and made the side-effect handler surface errors when persisting Unknown to the Effect Ledger fails.
- Production Worker allowlist remains empty/fail-closed. The new HTTP contract is not wired into production handler selection.

## Current uncommitted work

- Provider contract implementation and tests under `internal/effects/`.
- Side-effect ledger error propagation under `internal/worker/handlers/`.
- ADR-0005 and Wave 3 reporting documents are being prepared.
- Targeted Race tests for the new provider and side-effect paths have passed 10 consecutive runs; broader clean-commit validation and Evidence are pending.

## Remaining work and next commands

1. Finish Wave 3 documents and review Provider port semantics; run `go test -race ./internal/effects ./internal/worker/handlers -count=10`.
2. Create a scoped Wave 3 implementation commit, then run full required unit/Race/PostgreSQL/Failure Lab/Chaos/Vet gates against that exact clean commit and append a separate Evidence record.
3. Continue independent packages: cross-container ArtifactStore crash/response-loss E2E; API cursor rotation/OpenAPI coverage and shared-rate-limit ADR; trace/alerts/backup operational verification; image/SBOM/release readiness.
4. Preserve Production Fail-Closed. External IdP, real Provider/Sandbox, cloud S3/DR, production RPO/RTO, and SLO/on-call remain unverified.

Latest resumable command after checking status: `git status -sb; git rev-parse HEAD; go test -race ./internal/effects ./internal/worker/handlers -count=10`.
