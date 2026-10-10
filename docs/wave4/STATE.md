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

- Overall: `WAVE4_ENGINEERING_PARTIAL` (Package A local acceptance complete; Package B implementation is in local gates; Packages C–G remain).
- Current package: B — end-to-end distributed tracing, final validation.
- Completed: Package A committed as `e3d496495dd4c9da6cdfb67b923facda2910aed3`, tree `921e3098cfb13d712d7eaf7165f303a79713bc59`. It documents all 15 stable v1 operations, binds handler JSON through named DTOs, and tests router/method, DTO/schema, parameters, statuses, roles and error-envelope drift. See `WAVE4_PACKAGE_A_API_CONTRACT.md`.
- Validation: API unit/race/vet PASS; full `go test ./... -count=1` PASS; full `go test -race ./... -count=1` PASS; required PostgreSQL integration `go test ./internal/storage/postgres -count=1 -v` PASS on a fresh isolated PostgreSQL 16 instance (99.285s). The explicit backup/restore drill was SKIPPED by its opt-in guard and is not part of Package A acceptance. Final evidence and hashes: `E:\distributed-task-lab-temp\wave4-artifacts\package-a-final-e3d4964-20261010`.
- Package B in progress: fixed a production payload validation bug exposed by traced E2E (`_traceparent` injected by trusted API context was rejected as unknown RAG data); added reserved-metadata validation and regression tests. Worker execution now uses linked consumer spans rather than false synchronous parentage across queue wait; PostgreSQL state transitions and S3 operations have safe spans; ACK/failure writes retain the worker span under bounded detached context. The optional tracing harness starts the digest-pinned Collector, checks actual HTTP method/path attributes, polls exporter delivery with a bounded deadline, and redacts credentials.
- Package B E2E: two isolated successful runs so far, both from dirty source and clearly identified as such in manifests. The second includes a real two-worker crash after MinIO PUT/before ACK and confirms recovery on attempt 2 / lease version 2 with `expired_lease` telemetry. Earlier failed attempts are preserved under the E: tracing run directory and documented in `WAVE4_PACKAGE_B_DISTRIBUTED_TRACING.md`.
- Unresolved: full Race, required PostgreSQL integration, clean-commit repeat, and hosted CI for Package B. Packages C–G remain; production remains `PRODUCTION_NOT_APPROVED` and the Worker allowlist remains closed.
- Next: run full unit/race/vet and required fresh PostgreSQL integration; commit Package B if gates pass; repeat tracing + crash recovery on the clean commit and hash the final E: evidence. Then proceed to Package C.

## Recovery

```powershell
Set-Location -LiteralPath 'E:\distributed-task-lab-temp\wave4-artifacts\worktree'
git status --short --branch
git rev-parse HEAD
```

Continue from this file and `TASK_BOARD.md`. Do not reset or rewrite the Wave 3 branch, its evidence, or the Phase 0 tag.
