# P0-F — Local backup and restore safety design

Status: design and safety preflight only; no source DB or object store was accessed.

## Snapshot consistency

PostgreSQL and S3 snapshots are not one atomic transaction. Record both capture
timestamps and inventory hashes. A task row may be committed before its
artifact exists; an artifact may exist before fenced ACK; a live lease may
expire/reclaim during backup; an external effect may be `unknown`. Therefore a
restore report must classify tasks with missing/referenced artifacts, retain
all immutable artifacts found in the snapshot, and quarantine unknown effects
for reconciliation. Do not automatically replay them.

## Safe workflow

1. Preflight confirms a dedicated source run/project, destination database and
   destination bucket/prefix; source and destination must differ.
2. Use `pg_dump --format=custom --no-owner --no-acl` to a newly created E:
   run directory; never place DSNs or secrets in logs.
3. Inventory the exact source bucket/prefix and record object key, size,
   content type and locally computed SHA-256 where bytes can be streamed.
4. Seal manifest and logs only after all writers close; hash the closed files.
5. Restore only to a dedicated recovery database and a fresh bucket/prefix.
6. Verify migration/schema state, task/effect/event/audit counts, artifact
   references and byte hashes; report missing/unreferenced objects separately.
7. Preserve the original snapshot and evidence. No delete or overwrite path is
   part of this design.

`scripts/local-first/backup-preflight.ps1` now checks local-first source identity,
distinct recovery DB/bucket/prefix, E:-only output, collision avoidance, and
presence of existing PostgreSQL/object inventory tools. It exits nonzero with
structured failures and never connects to a database or object store.

`backup-postgres.ps1` is plan-only by default. With `-Execute`, it requires the
6 GiB container resource gate and exactly one PostgreSQL service container
whose Compose labels match the run ID; it dumps only database `tasklab` to a
new E: directory, reads the database password from the container-mounted
secret file, checks the `PGDMP` signature, and seals hashes. It is PostgreSQL
only and not consistent with the MinIO/object snapshot. The read-only
`verify-postgres-backup.ps1` checks dump signature, size, manifest digest, and
SHA256SUMS without restoring or modifying anything. Neither script was run
against a database in this checkpoint. They are harness implementation, not
backup acceptance.

## Status boundary

No backup script in this checkpoint was executed; the PostgreSQL backup and
verification harness is implemented but unverified. Object backup, restore
scripts, and isolated recovery were not implemented. `LOCAL_RESTORE_VERIFIED`
is not claimed. Full operational implementation remains a next offline package.
