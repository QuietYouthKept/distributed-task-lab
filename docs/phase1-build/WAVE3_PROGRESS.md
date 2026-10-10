# Wave 3 progress

Updated: 2026-10-10 (Asia/Shanghai)

## Recovery point

- Branch: `codex/phase1-production-hardening`.
- Latest pushed implementation commit: `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` (safe ingress bucket reclamation and ADR-0006).
- Evidence for `9b360f5`: `0f12b94dfa33cd6fb0ed8ea1a30692100541ddd5` (pushed); Evidence for `cedbdc1` is collected and awaiting its separate append-only Evidence commit.
- PR #1 remains OPEN and Draft.
- CI Run `37962333979`: PASS, including actual source-built MinIO image, S3 contract, API/two-Worker recovery, PostgreSQL backup/restore, PostgreSQL integration, full Race, and public Go Client module.
- Security Run `37962334001`: govulncheck, CodeQL, and Secret Scan PASS; Scorecard was conditionally SKIPPED.
- CI Run `37963245838` at `9b360f5`: all five jobs PASS. Security Run `37963245883`: govulncheck, CodeQL, Secret Scan PASS; Scorecard was conditionally SKIPPED.
- CI Run `37965020508` at `cedbdc1`: all five jobs PASS, including real MinIO image build/start, S3 contract, API/two-Worker process recovery, backup/restore, PostgreSQL Integration, full Race and public client. Security Run `37965020486`: govulncheck, CodeQL and Secret Scan PASS; Scorecard was conditionally SKIPPED.
- Prior failed runs `37956337334`, `37956336466`, and `37960805439` remain unchanged.
- Wave 2 Evidence `wave2-20261009T153618132Z-a292ec7a996b` stays bound to implementation `a292ec7a996bc39214ab413c420eacd9268a7298`; it does not validate new code.

## Completed in this continuation

- CI follow-up fix was pushed as `ac46b6b`; remote CI and Security results above are bound only to that SHA.
- The MinIO JSON checksum parser now accepts the trailing comma emitted by Go 1.26.9; failures report non-secret diagnostics. Module and license hashes are verified before image construction.
- `test/external-client/go.mod` now targets the root module's Go 1.26.9 toolchain.
- Began Wave 3 package B: added provider-independent HTTP Provider port and contract client with outcomes that distinguish not-sent, confirmed rejection/success, explicit retryable rejection, and unknown. Added HTTP contract/fault tests and made the side-effect handler surface errors when persisting Unknown to the Effect Ledger fails.
- Production Worker allowlist remains empty/fail-closed. The new HTTP contract is not wired into production handler selection.
- Clean local Evidence `wave3-20261010T171000Z-9b360f5952bb`: 14/14 PASS, exact implementation `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf`, clean start and clean after excluding the run artifacts. PostgreSQL 16.15, Docker 28.3.3, Go 1.26.9, temporary source-built MinIO image; all command-log hashes and benchmark/chaos artifact hashes are in the manifest.
- A first Evidence-wrapper preflight did not start tests because Git Bash `curl` lacked the local proxy and the license fetch was reset. The `NOT VERIFIED` preflight and observed log are preserved separately; retry through `127.0.0.1:7892` completed 14/14.

## Package C completed locally; Evidence commit pending

- Inspection found the 10,000-entry process-local ingress limiter never reclaimed inactive buckets; after enough distinct principals, new identities could remain rejected until process restart.
- Commit `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` reclaims only buckets whose normal refill calculation would already restore the full burst; no cluster-wide rate-limit semantics were introduced.
- Regression test checks just-before and exact full refill. Targeted Race ×100 and vet passed; full clean Evidence run `wave3-20261010T171600Z-cedbdc1edb09` passed 14/14 against the exact commit, with clean start and clean after excluding run artifacts. Manifest, raw logs, and hashed Chaos/pagination artifacts are preserved in that run directory.
- CI Run `37965020508` and Security Run `37965020486` both completed successfully for `cedbdc1`; Scorecard was conditionally skipped by its PR-event condition.
- ADR-0006 proposes, without enabling, shared-quota and cursor-key-rotation decision requirements. No cursor format/config changed.

## Current uncommitted Evidence only

- Package C source/test/ADR are committed and pushed in `cedbdc1`.
- New run `wave3-20261010T171600Z-cedbdc1edb09` and the append to `evidence/phase1/index.json` are uncommitted and must be committed separately from implementation.
- Package B source/tests are in implementation commit `9b360f5`, Evidence commit `0f12b94`; both are pushed and the clean Evidence is 14/14.

## Remaining work and next commands

1. Commit and push the collected append-only Evidence for `cedbdc1` separately from implementation.
2. Continue independent packages: cross-container ArtifactStore crash/response-loss E2E; OpenAPI schema coverage; full trace/alerts/backup operations; image/SBOM/release readiness.
3. Preserve Production Fail-Closed. External IdP, real Provider/Sandbox, cloud S3/DR, production RPO/RTO, and SLO/on-call remain unverified.

Latest resumable command after checking status: `git status -sb; git rev-parse HEAD; go test -race ./internal/api -run '^TestIngress' -count=100`.
