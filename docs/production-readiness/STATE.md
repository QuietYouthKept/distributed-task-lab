# Production-like Local State

Snapshot: 2026-10-11 (Asia/Shanghai). Local/offline checkpoint only; this is
not complete P0/P1 acceptance and does not grant production approval.

> Latest follow-up: implementation is now at `e2714f36b357146249b0fbc21d226f99125d6139`
> (tree `2498cab7a64ef38d0605ba6ab6bf5802ac6116f6`). The snapshot below is the
> earlier `366c320` checkpoint and is retained as historical context. For
> current results use `RUNTIME_UNBLOCKING_2026-10-11.md` and the regenerated
> verification matrix. Current P0/P1 remains partial; PostgreSQL runtime, Core
> E2E, backup/restore, OTLP/alert delivery, and multi-instance acceptance are
> not verified. Latest known container preflight is blocked at 4.04 GiB versus
> the 6 GiB threshold; no new container was started. Production remains
> `PRODUCTION_NOT_APPROVED`.

## Source and boundaries

- Worktree: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\worktree`
- Branch: `codex/production-readiness-p0-p1`
- Implementation commit: `366c320cbc60392de4cbe32f94b98d02c5cd9edd`
- Implementation tree: `ce6b4a5feb602e27bc71114b8fe7269970810420`
- A documentation/evidence-index closeout follows this implementation commit;
  functional verification below is bound to the implementation commit/tree.
- Evidence root: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\deep-engineering`
- New implementation and evidence-index work stayed on E:. D: repository,
  historical evidence, other Docker projects, and Migration 012 were untouched.
- No Push, Merge, Release, cloud resource, real production secret, formal
  Purge, Payment/Notification provider, or Production Worker execution.
- Production worker remains fail-closed; production allowlist empty; Purge
  disabled; `PRODUCTION_NOT_APPROVED`.

## Current implementation and verification state

- `P0_IMPLEMENTED: PARTIAL`; `P0_STATIC_VERIFIED: PARTIAL`.
- `P0_LOCAL_E2E_VERIFIED: NOT VERIFIED` — no service start, API token, task,
  database, MinIO/S3, backup restore, cross-process trace, or alert delivery
  was executed in this continuation.
- `P1_IMPLEMENTED: PARTIAL`; `P1_STATIC_VERIFIED: PARTIAL`.
- `P1_LOCAL_E2E_VERIFIED: NOT VERIFIED` — no multi-API PostgreSQL limiter,
  live cursor rotation, or DB/S3 Retention flow.
- `RESOURCE_LIMITED: YES` for container E2E: latest measured RAM was 4.02 GiB,
  below the 6 GiB container threshold. No new container was started. TIME_WAIT
  was 390 and E: free space was 49.5 GiB.
- `DEPENDENCY_BLOCKED: YES`: trusted MinIO registry digest remains unavailable;
  the official registry attempt ended in `401 insufficient_scope`. No local
  cache or alternate image was accepted as provenance.
- Scanner tools `govulncheck`, Trivy, Syft, and Cosign were not installed or
  run. No current-source vulnerability scan, SBOM, signature, or provenance
  was produced.
- `EXTERNAL_NOT_VERIFIED: YES`: hosted IdP, commercial providers, cloud,
  production DR/SLO/on-call and paid infrastructure remain outside scope.

## Post-commit offline evidence

All following runs were executed against implementation commit
`366c320cbc60392de4cbe32f94b98d02c5cd9edd`, tree
`ce6b4a5feb602e27bc71114b8fe7269970810420`, with `dirty_at_start=false` and an
empty source inventory. The matrix currently contains 88 run manifests.

| Suite | Run ID | Result |
|---|---|---|
| PowerShell static / `git diff --check` | `20261011T114644279Z` | PASS; 0 functional tests (syntax/static only) |
| Evidence integrity | `20261011T114644762Z` | 15/15 PASS |
| Scoped Compose cleanup / recovery-scope controls | `20261011T114646483Z` | 16/16 PASS |
| Database URL/toolchain safety | `20261011T114648096Z` | 9/9 PASS |
| Backup/restore manifest validation | `20261011T114649283Z` | 12/12 PASS |
| Supply-chain source static checks | `20261011T114650665Z` | 4/4 PASS |
| Retention archive fixtures | `20261011T114652003Z` | 12/12 PASS |

Evidence audit result: `FAIL`, 88 manifests inspected, 8 historical metadata
issues. The five earliest dirty records have no source inventory; three older
blocked records have no exact command. Their original logs and manifests are
preserved. All new runs are present in the matrix and have valid SHA256SUMS.
This audit finding does not invalidate the new offline test results, but the
whole historical evidence archive cannot be called clean.

See `VERIFICATION_MATRIX.json`, `VERIFICATION_MATRIX.md`, and
`EVIDENCE_INTEGRITY_ACCEPTANCE.md` for per-run hashes, counters, limitations,
and issue rows. Evidence results apply only to each recorded commit/tree and
test level.

## Next safe checkpoint

1. When resources meet the 6 GiB gate and an approved immutable MinIO source is
   available, run the separate PostgreSQL-only acceptance first, then one-task
   Compose E2E and collect a fresh current-source evidence bundle.
2. Before DB tests, provision only a disposable loopback database named
   `tasklab_local_test_admin` (or a permitted suffix) and pass its explicit
   maintenance URL to the guarded scripts.
3. For full historical evidence cleanliness, preserve and manually adjudicate
   the 8 listed gaps; do not rewrite or fabricate metadata for old records.
4. Keep production fail-closed until separate identity, provider, operational,
   supply-chain and recovery approval gates have real evidence.
