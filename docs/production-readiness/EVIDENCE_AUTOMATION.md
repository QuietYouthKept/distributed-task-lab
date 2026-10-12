# Evidence automation

`scripts/local-first/run-verification.ps1` writes evidence outside the source
tree under E:. It distinguishes `PASS`, `FAIL`, `NOT RUN`, `RESOURCE_LIMITED`,
`DEPENDENCY_BLOCKED`, and `IMPLEMENTED_NOT_VERIFIED`; Go test JSON events are
counted as executed/pass/fail/skip, and zero executed tests cannot become PASS.
Current suites include static validation, selected unit, guarded race,
integration preconditions, E2E resource guard, security-tool availability,
retention fixture, evidence-integrity self-tests, Compose cleanup fake-executor
tests, database URL safety tests, backup-manifest offline fixtures, static
supply-chain review, and a manifest-backed evidence audit. The audit checks
commit/tree correspondence,
dirty-source inventory hashes, exact commands/exit codes, counters, matrix
consistency, and every listed SHA256SUMS target. Historical records missing
metadata are reported as integrity gaps rather than rewritten.

For each run it records source commit/tree, dirty state, SHA-256 inventory of
changed/untracked source paths, exact command, exit code, tool version,
resource snapshot where applicable, limitations, and closed raw logs. It hashes
all run files (including raw logs and captured Go stdout/stderr), manifest, and
source inventory only after their writers have closed.
No secret values are captured from environment variables.

`update-verification-matrix.ps1` reads run manifests and generates
`VERIFICATION_MATRIX.json`; it updates a marked generated section in
`VERIFICATION_MATRIX.md` while retaining prior evidence rows. A PASS is scoped
to the exact source inventory and test level. The matrix is not production
approval and does not turn skipped integration tests into acceptance.

The PostgreSQL backup and retention fixture tools write their own E:-only
checksums for their generated output. Their manifests explicitly state when no
database/object service was contacted.
