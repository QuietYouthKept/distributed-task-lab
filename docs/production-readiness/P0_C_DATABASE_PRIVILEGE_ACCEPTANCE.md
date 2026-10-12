# P0-C — PostgreSQL privilege and disposable-test acceptance

Status: `OFFLINE_VERIFIED / LIVE_DATABASE_NOT_RUN`. The software test
preconditions are hardened, but exact grants and runtime behavior have not
been executed against PostgreSQL in this increment. Production approval is
not implied.

## Offline safety verification

The database test scripts now share `database-safety.psm1`. The URL validator
requires a PostgreSQL scheme, a loopback host, an explicit valid TCP port, an
explicit maintenance-role username, and a database named
`tasklab_local_test_admin` or a suffixed disposable variant. An omitted/default
port, remote host, unsafe database name, absent role, or invalid port is
rejected before Go starts. Test output is buffered and the supplied URL and
password are redacted before it is persisted. The URL itself is not written to
the manifest.

`test-database-safety.ps1` and the `database-safety` Evidence Runner suite cover
these input and redaction cases without contacting PostgreSQL. Latest current
source evidence is recorded in `VERIFICATION_MATRIX.json` once regenerated.

## Repository access classification

| Caller | Reads | Writes | Locking / fencing | Privileged SQL |
|---|---|---|---|---|
| API task repository | tasks, queues, task events, effects, audit rows, schema readiness | task/queue inserts and updates; events/audit append; effect creation/resolution; ingress bucket updates | transactions and row predicates; owner checks are application/API query constraints | none at runtime |
| Worker repository | eligible tasks, queues, workers, effects | task claim/lease/ACK/retry/DLQ, queue rate state, worker heartbeat, effect state, event append | `FOR UPDATE SKIP LOCKED`; lease-version and owner predicates on task transitions; transactions | none at runtime |
| Shared ingress limiter | limiter bucket | bucket insert/update | row lock and transaction | none at runtime |
| Retention dry-run | tasks/events/effects/artifact references | none | read-only snapshot semantics | none |
| Migrator | migration history and schema objects | migration history and DDL | migration advisory lock and transaction boundaries | DDL; migrator role only |

The deployed grants are in
`deployments/local-first/postgres/runtime-grants.sql`. API and Worker receive
separate table-level DML grants and no DELETE, TRUNCATE, schema CREATE, role
administration, or database administration. Events and audit tables are
append-only for runtime roles. Sequence privileges cover event/audit inserts.

## Critical authorization distinction

The application checks that protect lease owner, task state, attempt, and
`lease_version` are SQL predicates/transaction rules in the repository. They
are not automatically enforced by PostgreSQL merely because the role has
`UPDATE` permission. The current table-level grants allow a compromised API or
Worker credential to update any column on its granted tables, including
`lease_version`, lease fields, retry scheduling, queue governance, and effect
state. Existing CHECK constraints protect value shape/ranges, not transition
authorization. This remains a material residual risk.

Do not add column-level grants until every `UPDATE ... RETURNING`, conflict
update, and conditional assignment has been enumerated and tested. A SECURITY
DEFINER function would introduce its own escalation surface and is not a
minimal fix. The current least-privilege integration test also constructs
temporary grants in test code rather than applying the exact checked-in
three-role SQL; it does not yet prove retry, lease renewal, reclaim, DLQ, or
effect-ledger paths under those exact grants.

## Exact live command after an isolated database is provisioned

Both scripts require an explicit URI, port, user, and disposable maintenance
database. The test harness creates uniquely named temporary databases and must
not be pointed at any other local service:

```powershell
$env:TEST_DATABASE_URL = 'postgres://tasklab_local_test_admin:<secret>@127.0.0.1:<explicit-port>/tasklab_local_test_admin'
pwsh -NoProfile -File scripts/local-first/test-db-bootstrap.ps1 -TestDatabaseURL $env:TEST_DATABASE_URL
pwsh -NoProfile -File scripts/local-first/test-db-privileges.ps1 -TestDatabaseURL $env:TEST_DATABASE_URL
```

Do not put the real password in a shell transcript or command history; inject
it through a protected local process environment. These commands have not been
run. No database was connected, created, or dropped by the offline tests.
