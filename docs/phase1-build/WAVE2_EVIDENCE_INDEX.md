# Wave 2 evidence index

| Evidence ID | Status | Location | Scope |
|---|---|---|---|
| `wave2-chaos-20261009T092028Z` | PASS | `benchmarks/chaos/20261009T092028Z/{manifest.json,summary.json,events.jsonl,report.md}` (gitignored local artifact) | Six local PostgreSQL-backed failure scenarios, 30 trials each. It is not a full-system or MinIO run. |
| `wave2-unit-race-current-worktree` | PASS | Command outputs in this task execution; summarized in `WAVE2_VERIFICATION_MATRIX.md` | Unit/required PostgreSQL and full Race executed on a dirty worktree before an Implementation Commit. Not a clean-worktree Evidence Manifest. |
| `wave2-clean-run-collector` | PASS | `scripts/evidence/collect-wave2.ps1` | Committed; requires a clean start, records log hashes, config/dependency identity and snapshots newly created Chaos artifacts without mutating ignored source runs. |
| `wave2-20261009T093214275Z-0b73ce734dcd` | NOT VERIFIED | `evidence/phase1/runs/wave2-20261009T093214275Z-0b73ce734dcd/` | Clean-start commit `0b73ce7`; all seven executed commands passed; MinIO contract was explicitly NOT VERIFIED because endpoint/bucket/access/secret environment variables were absent. Manifest and raw logs committed in Evidence Commit `85cf764`. |
| `wave2-20261009T094212634Z-fcd9bf97ea83` | FAIL | `evidence/phase1/runs/wave2-20261009T094212634Z-fcd9bf97ea83/` | Preserved regression run: PostgreSQL Race `-count=5` failed in `TestMarkFailureWrongWorker` due to the immediate-due assumption in `claimOne`; other executed checks passed, MinIO remains NOT VERIFIED. Raw chaos artifacts are included. Evidence Commit `2eff38c`. |
| `wave2-20261009T095347192Z-f16c2df5831f` | FAIL | `evidence/phase1/runs/wave2-20261009T095347192Z-f16c2df5831f/` | Preserved full Race failure in `TestGlobalQueueRateLimitAcrossWorkersAndStableRefresh`: its 2-second DB-clock polling context expired during a PostgreSQL query. Of seven executed commands, the other six passed, including PostgreSQL Race ×5 and RetryBackoff ×20; MinIO is NOT VERIFIED. Raw Chaos artifacts are bundled. Evidence Commit `c882c3a`. |
| `wave2-rate-limit-race-repro-20` | PASS | Current execution output; rerun command recorded in `PROGRESS.md` | Test-only DB-clock token polling adjustment passed 20/20 targeted Race repetitions. This is not a clean-commit Evidence run. |
| `wave2-minio-contract` | NOT VERIFIED | No run directory | Credential variables were unavailable to this execution context; service health alone is not contract evidence. |
| `phase0-baseline` | Historical | `evidence/index.json` | Retained as historical evidence for its recorded Phase 0 commit only; not evidence for Wave 2. |

Do not edit or replace existing run data. The local Chaos directories are ignored by Git and remain in place; each failed run stays failed even after a test-only correction. Another clean-worktree Evidence Commit is still pending for the current test adjustment.
