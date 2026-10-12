# P1-C — Retention and archive boundary

The database-backed inventory remains read-only dry-run. A new offline manifest
compiler consumes a previously exported inventory snapshot and creates a
bounded archive manifest; it is not connected to PostgreSQL or S3. Formal task
purge and object deletion remain disabled. Existing PostgreSQL integration
tests exercise conservative terminal candidates and invalid bounds, but they
were not run in this checkpoint.

## Offline manifest contract

The compiler creates a stable bounded manifest, external SHA-256 file
checksum, run-scoped E: output, exclusion counters, and a Base64URL resume
cursor bound to policy version, inventory snapshot, and cutoff. It reapplies
eligibility rules on each resumed page. The cursor is not signed/authentication
material and must not authorize destructive actions.

Eligibility uses the exported PostgreSQL clock and requires `finished_at <
cutoff`; rows exactly at the cutoff are excluded, matching the live dry-run
query's strict boundary.

It rejects nonterminal/retrying tasks, active leases, DLQ items under
investigation, intent/unknown/unrecognized effects, duplicate IDs, and
malformed/incomplete artifact inventories. Task event and audit arrays are
preserved in each manifest record. All batches are bounded to 1,000 records.

Fixture test `scripts/local-first/test-retention-archive.ps1` passed 9 bounded
assertions: unsafe-candidate exclusion counts, audit/event preservation,
manifest validation and checksum, deterministic repeated-batch hash, resume
continuation, mismatched-snapshot resume rejection, and invalid artifact hash
fail-closed behavior. Evidence lives under the E: retention-fixture root.

## Limits

This does not fetch object bytes, verify hashes against S3, prove DB snapshot
consistency, nor connect to the production retention query. It is not an archive
restore writer. No DELETE SQL, object delete, or formal purge is enabled.

Status: `READ_ONLY_DRY_RUN + OFFLINE_MANIFEST_FIXTURE_VERIFIED`,
`LIVE_ARCHIVE_NOT_INTEGRATED`, `PURGE_DISABLED`.
