# Wave 4 execution state

Updated: 2026-10-10 (Asia/Shanghai)

## Baseline and branch

- Stable branch: `codex/phase1-production-hardening`
- Stable / remote verified HEAD: `a8656a7b72d792986775d94fe007acc7771c072b`
- Baseline tree: `14675a4cf091e87d6eb629b22d151eb35816fd92`
- Wave 4 branch: `codex/wave4-engineering-hardening`
- Wave 4 worktree: `E:\distributed-task-lab-temp\wave4-artifacts\worktree`
- Phase 0 tag: `upgrade-p0-complete-20261008` → `e66cf722c51b740243ae6d6126eea7fe361ada04` (unchanged)
- Initial state: clean worktree created from the verified stable HEAD; Package A is committed on the isolated Wave 4 branch.
- Production state: `PRODUCTION_NOT_APPROVED`; production Worker allowlist remains closed.

## Current phase

- Overall: `WAVE4_ENGINEERING_PARTIAL` (Packages A–C local acceptance complete; D candidate locally verified but awaits policy approval; E dry-run and benchmark locally verified but retention policy remains open; F local observability and graceful-drain acceptance complete with rollback/security limits; G source-grounded docs drafted).
- Current package: final acceptance and exact-SHA revalidation. Packages D and E candidates must not be promoted without decisions described in their ADRs.
- Completed: Package A committed as `e3d496495dd4c9da6cdfb67b923facda2910aed3`, tree `921e3098cfb13d712d7eaf7165f303a79713bc59`. It documents all 15 stable v1 operations, binds handler JSON through named DTOs, and tests router/method, DTO/schema, parameters, statuses, roles and error-envelope drift. See `WAVE4_PACKAGE_A_API_CONTRACT.md`.
- Validation: API unit/race/vet PASS; full `go test ./... -count=1` PASS; full `go test -race ./... -count=1` PASS; required PostgreSQL integration `go test ./internal/storage/postgres -count=1 -v` PASS on a fresh isolated PostgreSQL 16 instance (99.285s). The explicit backup/restore drill was SKIPPED by its opt-in guard and is not part of Package A acceptance. Final evidence and hashes: `E:\distributed-task-lab-temp\wave4-artifacts\package-a-final-e3d4964-20261010`.
- Package B accepted locally at `8099c68fd1448999cf3a262672519967d0a1d867` (tree `9eb8bde1820462f7fb3353810bc2bac20baa18df`). Full unit, full Race, vet, and required PostgreSQL integration passed; the explicit backup/restore drill was skipped because its opt-in flag was not set. Clean-commit Collector + crash/reclaim E2E passed with fresh credentials and per-file hashes. Details and caveats: `WAVE4_PACKAGE_B_DISTRIBUTED_TRACING.md`; evidence lives on E:.
- Package C accepted locally at `8e76d2f5a90b614a99f16cfa8b9d623a314c4667` (tree `5c7e81eb49683fed9a0b9ab3d46ccd91a309274c`). Full unit, full Race, vet, required PostgreSQL integration, stable request/effect identity checks, immutable receipts, provider response-loss reconciliation, and receipt-vs-manual-resolution Race passed. Backup/restore was opt-in and skipped. Detailed limits and E: evidence: `WAVE4_PACKAGE_C_EFFECT_RECONCILIATION.md`.
- Package D candidate implementation committed as `5258c5add74354809d53a9bf966775077fd492d5` (tree `2a36a69a90f411134de5f02cfb3a639d40772dfe`). Shared PostgreSQL ingress admission remains off by default; cursor v2 rotation is opt-in and v1 remains default. Two separately built API containers shared a burst of 16 (16 allowed / 48 rate-limited); strict PostgreSQL Race and API/config Race + vet passed. Adding migration 012 exposed and fixed a readiness migration-count constant lag. Policy approval and failure/contended-store testing remain open. Details: `WAVE4_PACKAGE_D_GLOBAL_ADMISSION.md`; E: run manifests/logs are listed there.
- Package E implementation commits: `983072bc8048d5cbc74beee21e94344d5fb0e648` (dry-run + skewed benchmark) and `cc1d9ccc88db0da9c0420d15927b557cdd8765b9` (full task-row EXPLAIN + overlap-index comparison). Retention remains read-only and no data was purged. Isolated PostgreSQL Race for retention and clean 100K pagination/EXPLAIN/index write-size benchmark passed. See `WAVE4_PACKAGE_E_LIFECYCLE_AND_PAGINATION.md` and E: evidence directories therein.
- Package F: commit `de5567f2f9e84a8bc8adfe5deb551db8c71f6eea` adds finite-cardinality API response, retry and artifact error metrics, development alert rules, and isolated E2E scripts. Targeted unit/Race passed; exact-SHA Prometheus E2E observed API scrape `NORMAL → FIRING → RESOLVED`. Exact-SHA release smoke observed API readiness and a Worker SIGTERM drain with an in-flight mock task succeeding at attempt 1 in 4.391s. Rollback across migration 012 remains blocked by newer-schema rejection. A fresh govulncheck attempt was blocked by EOF fetching the vulnerability database via proxy 7897; historical Wave 3 scans remain bound to their original SHAs. Details and evidence: `WAVE4_PACKAGE_F_OBSERVABILITY_RELEASE_SECURITY.md` and E: `alert-e2e-runs\alert-7b64f3230a82` / `release-smoke-runs\release-748866a0fa31`.
- Package G: added code-grounded system overview, critical call chains, and interview guide. Counts and measurements cite only prior Wave 4 evidence; all production claims are explicitly excluded. Files are listed below.
- Unresolved: hosted CI/security/SBOM scan for Wave 4 has not run. Package D policy approval and Package E retention/archive/artifact policy are not granted. The HTTP provider remains a tested contract, not a production-connected vendor adapter. Production stays `PRODUCTION_NOT_APPROVED` and the Worker allowlist stays closed.
- Next: finish the F audit and G documentation commit, then verify final tree/status. Do not push or alter PR/tags without user direction.

## Recovery

```powershell
Set-Location -LiteralPath 'E:\distributed-task-lab-temp\wave4-artifacts\worktree'
git status --short --branch
git rev-parse HEAD
```

Continue from this file and `TASK_BOARD.md`. Do not reset or rewrite the Wave 3 branch, its evidence, or the Phase 0 tag.

## Latest local closeout addendum (2026-10-10)

The earlier `Next` note above is superseded: F implementation is committed as `de5567f2f9e84a8bc8adfe5deb551db8c71f6eea`; its isolated alert and release smoke passed against that exact source commit. Full unit+PostgreSQL, full Race+PostgreSQL, Failure Lab 12/12, vet and diff-check passed at that SHA; matrix evidence is `E:\distributed-task-lab-temp\wave4-artifacts\final-matrix-de5567f-retry-20261010`. This closeout additionally source-built the pinned MinIO revision and passed authenticated S3 contract, API/two-Worker crash recovery, and PostgreSQL 16.15 backup/restore with artifact verification (`E:\distributed-task-lab-temp\wave4-artifacts\minio-backup-w4-ba99782851\acceptance`; application source `6c9763033fb394fc41e9071c24f284709bcc6a61`). Old/new API+Worker process tests proved schema11 works for Wave 3, schema12 works for Wave 4, and Wave 3 correctly fails closed on schema12 (`E:\distributed-task-lab-temp\wave4-artifacts\migration-runtime-matrix-3fca936`). Local govulncheck `./...` on Go 1.26.9 passed via proxy 7892 with zero reachable findings; `GO-2026-5932` remains unresolved at module level (`E:\distributed-task-lab-temp\wave4-artifacts\security-audit-6c97630-7892`). Exact code commit `3fca9369672bc7a5953b1a23b4c2447cf2d7d0a2` passed full unit/integration and Race/integration, RetryBackoff Race count 20, PostgreSQL Race count 5, Failure Lab 12/12. Logs/checksums are under `E:\distributed-task-lab-temp\wave4-artifacts\final-verification-3fca936`. Package G documentation exists at `architecture/SYSTEM_OVERVIEW.md`, `call-chains/CRITICAL_CALL_CHAINS.md`, and `interview/INTERVIEW_GUIDE.md`. Hosted final-SHA checks remain pending; rollback, policy approvals and production are not approved.
