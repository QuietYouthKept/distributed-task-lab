# P0-C — Migration and runtime database privileges

Status: `IMPLEMENTED_STATIC / POSTGRES_INTEGRATION_NOT_RUN`.

## Roles and flow

The local PostgreSQL init script creates three non-superuser logins from
per-run mounted secret files:

| Role | Use | DDL | Runtime data access |
|---|---|---|---|
| `tasklab_migrator` | Separate one-shot `cmd/migrate` | Schema owner; `CREATE` only on the isolated `public` schema | Migration metadata and objects it owns |
| `tasklab_api` | HTTPS API | None | Task submission/read/admin mutation, queue config, events, effects, append-only audit, optional shared limiter |
| `tasklab_worker` | Worker process | None | Claim/lease/heartbeat/ACK, events, queue scheduling state, effects |

All logins are `NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION`. Database
`CONNECT` is explicitly granted to the three roles after revoking PUBLIC
database privileges. API and Worker do not receive `DELETE`, schema `CREATE`,
or access to database administration.

The sequence of jobs is Postgres healthy → migrator completes →
`db-runtime-grants` connects as the migrator owner and grants only required
table/sequence DML → API/Worker start. The grants are explicit and repeatable
for the current schema inventory; future migrations adding runtime tables must
update this grant file and the privilege tests before release.

## Grant review notes

- API needs task/queue reads and controlled writes, task-event reads/inserts,
  effect reconciliation reads/updates, audit append/read, and shared limiter
  bucket operations. Audit UPDATE/DELETE are intentionally absent.
- Worker needs task reads/updates, event inserts, queue scheduling reads and
  updates, worker heartbeat writes, and effect ledger writes. Task deletion and
  audit writes are absent.
- Both runtime roles only read `schema_migrations` for readiness.
- The grant list intentionally includes no table ownership or `GRANT OPTION`.
- These are table-level grants, not column-level authorization. For example,
  a compromised API account with `UPDATE tasks` is not prevented by PostgreSQL
  from altering a lease/fencing column directly. Treat this as a local test
  boundary, not a complete defense against a compromised runtime process; a
  production-grade field-level boundary may require constrained SQL
  functions/views or another reviewed design.

## Validation still required

No PostgreSQL container was started because the current host is below the 6
GiB container gate. SQL syntax, exact current-query coverage, repeated
migration, checksum/schema-ahead rejection, DDL denial, API/Worker operation
success, and database-target isolation are not yet evidenced against this
bootstrap. The historical `least_privilege_integration_test.go` creates roles
manually and is not evidence that this bootstrap works.

Static Compose/topology, shell syntax, and role-grant boundary checks passed on
`f33c9c0b91c9c46f832adef6e1ab1a402859a6fc`; sealed evidence is
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\p0-runtime-static-20261011T135136+0800`.
This does not establish successful SQL execution or least-privilege runtime
behavior.

Migration 012 was not edited. The production Worker allowlist and production
fail-closed validation were not changed.
