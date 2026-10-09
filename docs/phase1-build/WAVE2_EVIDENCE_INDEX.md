# Wave 2 evidence index

| Evidence ID | Status | Location | Scope |
|---|---|---|---|
| `wave2-chaos-20261009T092028Z` | PASS | `benchmarks/chaos/20261009T092028Z/{manifest.json,summary.json,events.jsonl,report.md}` (gitignored local artifact) | Six local PostgreSQL-backed failure scenarios, 30 trials each. It is not a full-system or MinIO run. |
| `wave2-unit-race-current-worktree` | PASS | Command outputs in this task execution; summarized in `WAVE2_VERIFICATION_MATRIX.md` | Unit/required PostgreSQL and full Race executed on a dirty worktree before an Implementation Commit. Not a clean-worktree Evidence Manifest. |
| `wave2-clean-run-collector` | READY | `scripts/evidence/collect-wave2.ps1` | Must be committed, then run from a clean checkout. It writes a separate append-only Phase 1 index and preserves the Phase 0 index. |
| `wave2-minio-contract` | NOT VERIFIED | No run directory | Credential variables were unavailable to this execution context; service health alone is not contract evidence. |
| `phase0-baseline` | Historical | `evidence/index.json` | Retained as historical evidence for its recorded Phase 0 commit only; not evidence for Wave 2. |

Do not edit or replace existing run data. The local Chaos directories are ignored by Git and remain in place; an immutable, schema-complete Phase 1 Evidence Commit is still pending clean-worktree verification.
