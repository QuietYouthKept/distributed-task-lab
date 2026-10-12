# P0/P1 Local-First Status

Date: 2026-10-11 (Asia/Shanghai)

## Scope and source identity

- Worktree: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\worktree`
- Branch: `codex/production-readiness-p0-p1`
- Wave 4 source baseline: `da51f58ebe483f9f04577e160f59aaee3a8d2e7b`
- Latest local runtime implementation: `f33c9c0b91c9c46f832adef6e1ab1a402859a6fc` (tree `90be9e2f974da4e20f9e9d3cbe8fe018f274ef0c`).
- Clean-source static evidence: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\p0-runtime-static-20261011T135136+0800`.
- Validation in this document applies only to the implementation commit recorded with its evidence. It does not update or replace historical Wave 3/Wave 4/Benchmark evidence.
- All external provider, hosted identity, paid cloud, and production deployment claims remain unverified.

## Implemented in this local-first increment

- Added a `staging-sim` execution policy that admits only `eval/gateway`; the production policy remains empty and fail-closed.
- Split API and Worker profile validation so the API does not need MinIO or Gateway secrets and the Worker does not need OIDC/API TLS private configuration.
- Required local HTTPS identity/JWKS, explicit role mappings, a mounted local CA bundle, and API TLS files for `staging-sim`; the API JWKS client verifies the supplied CA and keeps TLS verification enabled.
- Added a local-only HTTPS JWKS/JWT protocol fixture with fixed test identities and opt-in test-token issuance, a non-root fixture Dockerfile, and an E:-only certificate generator that creates distinct DNS SANs for `identity` and `api`.
- Added mounted-secret-file support for the supported application secrets, with conflict rejection, sanitized errors, and inclusion in log redaction.
- Added `AUTO_MIGRATE`; the default remains convenient for development, while production and `staging-sim` require it to be false.
- Added a standalone `cmd/migrate` executable. This local-first implementation accepts only `APP_PROFILE=staging-sim`, the dedicated `tasklab_migrator` identity, and the isolated `postgres:5432` endpoint. It does not claim production migration authorization.
- Added a bounded local Gateway Simulator for the `staging-sim` eval contract. It has authenticated requests, request idempotency, bounded in-memory state, and explicit rate-limit/timeout/response-loss modes. Its results are simulated and cost is always zero.
- Expanded the simulator contract fixtures with explicit case failure, HTTP 500, malformed JSON, invalid token usage, and excessive simulated cost modes. Added an explicit test documenting that idempotency is lost on simulator restart.
- Added non-root, pinned-base application and Gateway Simulator Dockerfiles. The application Dockerfile builds only the selected API/Worker/migration executable and follows the target architecture. `.dockerignore` excludes env files, secret directories, PEMs, and private keys.
- Added a resource preflight script with separate unit, compile, race, and container thresholds. It reports existing containers but never removes or changes them.
- Added isolated Core Compose topology for PostgreSQL 16, one-shot Migrator, explicit post-migration runtime grants, MinIO, the local HTTPS JWKS fixture, API, Worker, and Gateway Simulator. Host ports are loopback-only; the Compose network is internal; Postgres and MinIO have scoped volumes, resource caps, and health checks.
- Added E:-only per-run credential/certificate provisioning. API and identity certificates are separate mounted bundles; the CA signing key is not mounted. Provisioning refuses overwrite and outputs paths, not credentials.
- Added non-superuser `tasklab_migrator`, `tasklab_api`, and `tasklab_worker` bootstrap plus explicit table/sequence grants; runtime DDL/DELETE are absent. SQL has syntax checks but has not executed against PostgreSQL.
- Added worker-only scoped MinIO user/policy bootstrap. MinIO image use is blocked pending a verified official registry digest: upstream is archived and Docker Hub/Quay manifest requests returned 401 through the configured proxy. No cache or mirror was used.

## Verified in this increment

Commands executed with Go `go1.26.9`, `GOMAXPROCS=2`, `GOFLAGS=-p=1`:

- `go test ./cmd/api ./cmd/migrate ./cmd/gateway-simulator ./internal/auth ./internal/config ./internal/domain/taskpayload ./internal/api ./internal/worker/handlers -count=1` — PASS.
- `go test -race ./cmd/gateway-simulator ./cmd/migrate ./internal/auth ./internal/config -count=1` — PASS.
- `go test -race ./internal/api ./internal/domain/taskpayload ./internal/worker/handlers -count=1` — PASS.
- `docker build --pull=false --tag tasklab-local-migrate:phase1-p0p1 --build-arg APP_COMMAND=migrate --build-arg SOURCE_COMMIT=working-tree -f deployments/Dockerfile.app .` — PASS; image is local only, non-root `tasklab`, no container started.
- `git diff --check` — PASS.
- Resource script snapshots: unit/race allowed; container profile blocked after build at 5.24 GiB available RAM (threshold 6 GiB). E: had 57.15 GiB free, TIME_WAIT remained below 1,000, and Docker Engine was available.

## P0 state

**P0_LOCAL_IMPLEMENTED: PARTIAL.** The profile boundary, local simulation policy, Gateway simulator, secret separation, local CA trust, standalone migration executable, and image build path are implemented.

**P0_LOCAL_VERIFIED: STATIC_AND_UNIT_PARTIAL.** Compose config and 29 topology assertions, TLS CA/SAN checks, shell/PowerShell syntax, policy JSON, focused Go packages, and SHA256 sealing passed. No image was built or service started; OIDC-to-Fenced-ACK E2E, API/Worker crash recovery, current-source PostgreSQL least-privilege acceptance, backup/restore, and API-to-OTLP remain unverified. Evidence measured 4.82 GiB available RAM (<6 GiB), so container E2E was not launched. MinIO official image manifest remains unresolved.

The P0-D contract-specific additions are documented in `P0_D_GATEWAY_CONTRACT.md`. On 2026-10-11, `cmd/gateway-simulator` and `internal/worker/handlers` tests passed under Go 1.26.9; the follow-up source commit and test logs are recorded in its external Evidence Manifest. They are unit/HTTP-handler evidence only; they do not change the overall P0 status or establish cross-process/restart guarantees.

## P1 state

P1 capabilities already present in the Wave 4 source (shared rate limiter, cursor key rotation, retention safeguards, role/tenant enforcement, audit, and supply-chain workflows) were not re-accepted as a complete P1 local program by the tests above. Their existing behavior must be re-audited and run against the current implementation commit. Retention/Purge remains disabled by default.

P1-B cursor rotation has since received focused local unit verification, including key overlap/cutoff and a second-codec compatibility check; this does not establish multi-process key distribution. See `P1_B_CURSOR_ROTATION.md` and the follow-up Evidence Manifest.

P1-A global admission remains opt-in and has only router/config unit coverage for this local-first source increment. The PostgreSQL shared-instance integration test has not been rerun against an isolated database at the current SHA. See `P1_A_GLOBAL_ADMISSION.md`.

P0-B now has a local JWKS fixture, RSA key rotation within the fixture, TLS trust tests, and scoped certificate provisioning. It remains fixture-level verification only; there is no Compose/API security matrix or complete OIDC login flow. See `P0_B_IDENTITY_E2E.md`.

P1-E auth/API negative unit packages have now passed on the verification-matrix source commit. Cross-owner database operations, audit persistence, and full security-governance acceptance remain unverified. See `P1_E_SECURITY_GOVERNANCE.md`.

P1-D has one additional verified local artifact: the identity fixture image built successfully with non-root metadata and pinned base references. It was not run, scanned, SBOM-generated, signed, or pushed. See `P1_D_SUPPLY_CHAIN.md`.

## Production gate

`PRODUCTION_NOT_APPROVED` remains the only valid production state. `staging-sim` is a local simulation profile, not production approval. Real provider behavior, hosted IdP operations, production TLS/key management, cloud backup/restore, cross-region recovery, production alert delivery/on-call, and real performance/RPO/RTO remain `NOT VERIFIED` or externally blocked.

## Next bounded work

1. Resolve an approved, fetchable official MinIO image digest (or stop for user authorization before using a third-party mirror).
2. When available RAM is at least 6 GiB, run Compose startup and PostgreSQL role/migration tests first; then verify API ownership/auth, Worker gateway call, MinIO artifact SHA and fenced ACK.
3. Add isolated-target PostgreSQL + MinIO backup/restore verification and migration compatibility checks without modifying Migration 012.
4. Close API/Worker OTLP and local alert receiver flow; label all provider metrics `SIMULATED`.
5. Re-run and record P1 limiter, cursor, retention, tenant/RBAC/audit, and supply-chain acceptance against the final implementation SHA.
