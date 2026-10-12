# P0-C — PostgreSQL runtime privilege audit

Status: source review complete; live database acceptance not run in this
checkpoint. Migration 012 is unchanged.

## Role model

`tasklab_migrator` owns/creates the schema objects required by migrations and
is a separate one-shot role. `tasklab_api` and `tasklab_worker` are created as
`NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION`, receive `USAGE` but not
`CREATE` on `public`, and receive DML grants after migrations. Runtime roles
have no DELETE grant, no schema DDL, and no role/database administration.
Sequence grants cover event/audit inserts.

## SQL access inventory (source-based)

The main repository SQL paths include:

| Component | Reads | Writes | DDL / privileged operations |
|---|---|---|---|
| API task/queue repositories | tasks, queues, events, effects, migrations | task/queue insert/update, task events insert, effects insert/update, audit insert, shared ingress bucket update | none at runtime |
| Worker claim/lease/ack/retry/DLQ | tasks, queues, workers, effects | task/queue/worker/effect update, task event insert | none at runtime |
| Shared ingress limiter | fixed slot table | bucket insert/update under transactions and row locks | first-use slot initialization inserts only |
| Retention dry-run | tasks/events/effects/artifact references | none | none |
| `cmd/migrate` | migration history | migration history/schema objects | migration DDL, migrator only |

This is a code-path inventory. A bounded lexical scan of 31 `*.go`/`*.sql`
files under `internal/storage/postgres` plus `cmd/migrate` found these SQL-word
occurrences (including tests, comments, identifiers in fixtures, and DDL; they
are **not** counts of executed production statements):

| Token | Occurrences |
|---|---:|
| SELECT | 83 |
| INSERT | 26 |
| UPDATE | 62 |
| DELETE | 8 |
| TRUNCATE | 2 |
| CREATE | 50 |
| ALTER | 32 |
| DROP | 17 |
| GRANT / REVOKE | 8 / 0 |
| NEXTVAL / SETVAL | 0 / 0 |
| EXECUTE / FUNCTION / SEQUENCE | 3 / 8 / 1 |

These are lexical inventory counts, not an AST-aware statement classifier.
Dynamic SQL and migration functions still require manual classification.

## Important residual risk: table-level UPDATE

The current grants use table-level UPDATE on `tasks`, `queues`, `effects`,
`workers`, and the ingress bucket table. PostgreSQL table-level UPDATE permits
updates to any column of those tables, even when application code only changes
a subset. Therefore the grants do **not** independently protect `lease_version`,
lease owner/expiry, retry schedule, effect state, or audit-sensitive fields
from a compromised runtime credential. Fencing correctness currently depends
on repository predicates and transactions, not a DB column permission fence.

`security_audit_events` and `task_events` have INSERT/SELECT but no UPDATE or
DELETE for runtime roles. That is a meaningful append-only boundary. RLS is not
needed to express the present owner checks because they are enforced in the API
query predicates, but live cross-owner database tests are still required.

Before narrowing UPDATE grants, enumerate every `UPDATE ... RETURNING` and
`INSERT ... ON CONFLICT DO UPDATE` column in queue/task/effect/worker/limiter
repositories. Column-level grants can be a reasonable next step, but they can
break statements that update a column conditionally. A bounded database
function would add SECURITY DEFINER risks and is not recommended without a
clear need and dedicated review.

## Existing integration coverage

`internal/storage/postgres/least_privilege_integration_test.go`
(`TestRuntimeRoleLeastPrivilege`) verifies a real temporary role can enqueue,
claim, ACK, and emit queue audit events, while rejecting DDL, DELETE, TRUNCATE,
role creation, event rewriting, and audit rewriting. It does not execute the
checked-in exact three-role grant SQL, does not cover retry/DLQ/lease renewal
with those grants, and does not prove per-column isolation. Tests skipped because
the database is absent are not acceptance PASS.

## Added harness

`scripts/local-first/test-db-bootstrap.ps1` runs the migration integration
matrix with `REQUIRE_TEST_DATABASE=1`; `test-db-privileges.ps1` runs the
runtime-role integration. Both now require an explicit loopback URL with a TCP
port, maintenance username, and database named `tasklab_local_test_admin`
(optionally with a suffix); logs are redacted before persistence. Their shared
offline safety rules are covered by `test-database-safety.ps1`. Exact grants
and live PostgreSQL flows remain unverified. See
`P0_C_DATABASE_PRIVILEGE_ACCEPTANCE.md` for residual table-level UPDATE risk.
