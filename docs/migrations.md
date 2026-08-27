# Migrations

Migrations are embedded, forward-only, ordered by numeric filename, and tracked
in `schema_migrations(version, name, checksum, applied_at)`. An applied file must
never be edited: checksum mismatch stops startup.

The runner uses one dedicated connection, a bounded `pg_try_advisory_lock` loop,
and a transaction per migration with lock/statement timeouts. DDL plus tracking
row commit together, so interruption is safely rerunnable. Concurrent API and
worker startup serializes on the advisory lock.

Migration 007 adds queue referential integrity, write-time checks, BIGINT event
IDs, shared rate state, and partial claim indexes. Constraints are `NOT VALID` to
avoid rewriting or silently changing old rows; they protect new/updated rows.
Operators should audit old data and `VALIDATE CONSTRAINT` during maintenance.

Rollback SQL is intentionally not generated. Restore from a tested backup or
ship a corrective forward migration.
