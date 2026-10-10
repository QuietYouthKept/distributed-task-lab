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

- Overall: `WAVE4_ENGINEERING_PARTIAL` (Package A local acceptance complete; Packages B–G remain).
- Current package: B — end-to-end distributed tracing.
- Completed: Package A committed as `e3d496495dd4c9da6cdfb67b923facda2910aed3`, tree `921e3098cfb13d712d7eaf7165f303a79713bc59`. It documents all 15 stable v1 operations, binds handler JSON through named DTOs, and tests router/method, DTO/schema, parameters, statuses, roles and error-envelope drift. See `WAVE4_PACKAGE_A_API_CONTRACT.md`.
- Validation: API unit/race/vet PASS; full `go test ./... -count=1` PASS; full `go test -race ./... -count=1` PASS; required PostgreSQL integration `go test ./internal/storage/postgres -count=1 -v` PASS on a fresh isolated PostgreSQL 16 instance (99.285s). The explicit backup/restore drill was SKIPPED by its opt-in guard and is not part of Package A acceptance. Final evidence and hashes: `E:\distributed-task-lab-temp\wave4-artifacts\package-a-final-e3d4964-20261010`.
- Unresolved: hosted CI for the Wave 4 commit has not run. Packages B–G are not accepted; production remains `PRODUCTION_NOT_APPROVED` and the Worker allowlist remains closed.
- Next: finish tracing call-chain and redaction audit; implement only missing spans/context relationships; add an isolated fresh-run Collector E2E and retain retry/crash evidence separately.

## Recovery

```powershell
Set-Location -LiteralPath 'E:\distributed-task-lab-temp\wave4-artifacts\worktree'
git status --short --branch
git rev-parse HEAD
```

Continue from this file and `TASK_BOARD.md`. Do not reset or rewrite the Wave 3 branch, its evidence, or the Phase 0 tag.
