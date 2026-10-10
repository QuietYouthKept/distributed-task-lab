# Incomplete Evidence Collection

- Run ID: `wave2-20261009T125557090Z-e12440cf94a9`
- Intended implementation commit: `e12440cf94a9`
- Collection status: `NOT VERIFIED` (no manifest or exit-code record was finalized).
- Failure: the current PowerShell host did not provide `Get-FileHash`; collection stopped while hashing copied Chaos Matrix artifacts. The raw stdout/stderr logs and copied event file are preserved unchanged.
- Independently confirmed from the raw Chaos Matrix artifacts: the command exited non-zero. Its run `20261009T125859Z` recorded intermittent claim/reclaim failures and cascading stale-task failures; this is a failed harness run, not production scheduling evidence.
- Container: the wrapper's temporary pinned MinIO container was stopped by its `finally` cleanup. It used no persistent volume; its random credentials and temporary environment file were removed. Existing containers were not altered.
- Follow-up: replace the PowerShell-only hash dependency with .NET SHA-256 and make the Chaos Matrix poll PostgreSQL-authoritative eligibility within a bounded deadline while cleaning each isolated trial task. Re-run with a new Run ID; do not alter this directory.
