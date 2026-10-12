# P0-F — Backup / Restore harness acceptance

Status: `OFFLINE_HARNESS_VERIFIED`; PostgreSQL dump, object-store copy, and
independent restore remain `NOT VERIFIED`. No cloud, database, or MinIO
resource was contacted. Production DR/RPO/RTO is not claimed.

## Implemented offline contract

`scripts/local-first/backup-manifest.psm1` provides a strict inventory parser
for an externally captured object inventory. Each row must bind bucket, key,
size, SHA-256, referencing task, artifact role, snapshot time, and backup Run
ID. It rejects duplicate bucket/key records, missing objects, byte size and
hash mismatch, unsafe paths/reparse points, out-of-scope prefixes, mismatched
Run IDs, and timestamps later than the declared snapshot boundary. Prefix
checks use a path boundary (`prefix/`) to prevent `run-7` from accepting
`run-70`.

The restore manifest validator requires a supported manifest/schema version,
distinct source and target database names, and a destination bucket/prefix
that does not equal or overlap the source scope. It does not create a database,
bucket, or object. It cannot establish that a remote bucket is empty or owned;
that must be checked against an explicitly isolated target immediately before
an eventual restore operation.

`test-backup-manifest.ps1` exercises valid inventory, missing object,
duplicate key, checksum mismatch, unsafe path, source-prefix boundary,
snapshot boundary, same-database refusal, overlapping destination refusal,
schema mismatch, and missing-manifest refusal. Latest offline fixture result:
12/12 PASS. Fixture object bytes are deleted only from the test run's own
temporary E: evidence subtree after assertions; the persisted test result and
fixture manifests are checksummed. This is `OFFLINE_PASS`, not a live backup or
restore.

## Existing PostgreSQL harness

`backup-postgres.ps1` and `verify-postgres-backup.ps1` remain explicit-opt-in
and PostgreSQL-only. No actual dump or restore was performed. The new inventory
validator is not yet connected to MinIO/S3 download/copy. A complete restore
must additionally verify task/effect/event/audit counts, referenced artifact
bytes, task state consistency, and quarantined unknown external effects.

## Live execution blockers

- MinIO remains `MINIO_IMAGE_SOURCE_BLOCKED` pending new trusted registry
  authorization/source evidence; the object service was not changed or
  substituted.
- No independent disposable source/recovery PostgreSQL and isolated empty
  target bucket/prefix were provisioned.
- No real snapshot consistency boundary across PostgreSQL and object storage
  has been measured. These captures are not atomic.
- Restore latency and recovery-point/recovery-time objectives remain
  unmeasured.
