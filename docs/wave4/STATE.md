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

- Overall: `WAVE4_ENGINEERING_PARTIAL` (Packages A and B local acceptance complete; Packages C–G remain).
- Current package: C — idempotency integrity and provider reconciliation.
- Completed: Package A committed as `e3d496495dd4c9da6cdfb67b923facda2910aed3`, tree `921e3098cfb13d712d7eaf7165f303a79713bc59`. It documents all 15 stable v1 operations, binds handler JSON through named DTOs, and tests router/method, DTO/schema, parameters, statuses, roles and error-envelope drift. See `WAVE4_PACKAGE_A_API_CONTRACT.md`.
- Validation: API unit/race/vet PASS; full `go test ./... -count=1` PASS; full `go test -race ./... -count=1` PASS; required PostgreSQL integration `go test ./internal/storage/postgres -count=1 -v` PASS on a fresh isolated PostgreSQL 16 instance (99.285s). The explicit backup/restore drill was SKIPPED by its opt-in guard and is not part of Package A acceptance. Final evidence and hashes: `E:\distributed-task-lab-temp\wave4-artifacts\package-a-final-e3d4964-20261010`.
- Package B accepted locally at `8099c68fd1448999cf3a262672519967d0a1d867` (tree `9eb8bde1820462f7fb3353810bc2bac20baa18df`). Full unit, full Race, vet, and required PostgreSQL integration passed; the explicit backup/restore drill was skipped because its opt-in flag was not set. Clean-commit Collector + crash/reclaim E2E passed with fresh credentials and per-file hashes. Details and caveats: `WAVE4_PACKAGE_B_DISTRIBUTED_TRACING.md`; evidence lives on E:.
- Unresolved: hosted CI for Wave 4 has not run. Packages C–G remain; production stays `PRODUCTION_NOT_APPROVED` and the Worker allowlist stays closed.
- Next: audit Package C's effect identity/fingerprint and provider reconciliation invariants; implement only gaps confirmed by source/tests; use isolated provider fault tests and preserve E: evidence.

## Recovery

```powershell
Set-Location -LiteralPath 'E:\distributed-task-lab-temp\wave4-artifacts\worktree'
git status --short --branch
git rev-parse HEAD
```

Continue from this file and `TASK_BOARD.md`. Do not reset or rewrite the Wave 3 branch, its evidence, or the Phase 0 tag.
