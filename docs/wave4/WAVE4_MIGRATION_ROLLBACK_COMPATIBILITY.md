# Wave 4 migration 012 and binary rollback compatibility

Status: `ROLLBACK_BLOCKED`. Do not roll Wave 4 binaries back to the Wave 3 baseline after schema 12 is applied. No rollback SQL is authorized or tested.

## Sources and schema change

- Wave 3 stable baseline: `a8656a7b72d792986775d94fe007acc7771c072b`, schema migrations 001–011.
- Wave 4 source tested: `3fca9369672bc7a5953b1a23b4c2447cf2d7d0a2` (migration/runtime matrix was executed against its Go application code; migration 012 itself is unchanged from `5258c5a`).
- `internal/storage/postgres/migrations/012_shared_ingress_rate_limit.sql` only creates `api_ingress_rate_limit_buckets` and a partial unique index. It does not alter task/effect tables, rewrite existing rows, or add a dependency to the pre-existing task schema. The limiter is optional and disabled by default.

The SQL is additive, but that alone does not make binary rollback compatible. Both API and Worker call `RunMigrations` before serving or becoming ready. Wave 3 embeds 11 migrations and `RunMigrationsContext` rejects an applied history longer than its embedded set (`internal/storage/postgres/db.go`, newer-schema guard). If startup got past that guard, its health check also requires exactly version/count 11 (`internal/storage/postgres/task_repository.go`, `latestSchemaVersion`). Wave 4 embeds 12 and readiness requires exactly 12.

### Compatibility dimensions

- **A — revision guard:** confirmed root cause for the observed start failure. The old binary fails in `RunMigrations` before it can serve or become ready.
- **B — direct SQL/schema mismatch:** migration 012 does not alter old tables/columns/constraints; source inspection found the old task SQL uses no new table. This is evidence of additive shape, not a proof that every old query works against every future schema.
- **C — data semantics:** migration 012 contains no task/effect data transformation. The matrix used clean databases and did not migrate a populated Wave 3 task/effect set, so cross-version read/write semantics for previously stored records remain unproven.
- **D — Worker execution:** the old Worker cannot start on schema12, so claim/renew/ACK/retry/DLQ/fail-closed behavior on schema12 was not and cannot be exercised with this binary. The tested matrix only proves process startup/readiness; it is not a task-operation matrix.

## Isolated process matrix

Runtime evidence is in `E:\distributed-task-lab-temp\wave4-artifacts\migration-runtime-matrix-3fca936`; source snapshots, earlier failed attempts, logs, binaries, and checksums are retained under adjacent `migration-matrix-*` directories. The tested old source was archived from the stated Wave 3 SHA. Each case used an isolated PostgreSQL 16 container and a fresh database. Readiness means HTTP 200 for both API and Worker; the two expected Wave 3 rejections exited with code 1 and recorded the specific newer-schema guard.

| Binary | Schema 11 | Schema 12 |
|---|---|---|
| Wave 3 API | PASS; starts and ready | BLOCKED as expected; exits before serving: schema 12 newer than embedded 11 |
| Wave 3 Worker | PASS; starts and ready | BLOCKED as expected; exits before serving: schema 12 newer than embedded 11 |
| Wave 4 API | PASS; startup applies 012, then ready at 12 | PASS; ready at 12 |
| Wave 4 Worker | PASS; startup applies 012, then ready at 12 | PASS; ready at 12 |

Thus the verified matrix confirms a real rollback boundary. It does not establish that old binaries can safely execute against schema 12, and it does not justify deleting the additive table or editing `schema_migrations` to force startup. The original migration 012 was not modified. Initial harness attempts (an incorrectly formed test URL, then a finalizer error after matrix output) remain retained separately; the final matrix and checksum manifest are the accepted run. Full Wave 4 unit/integration and Race suites validate current functionality on schema12, while historical Wave 3 evidence is SHA-bound and is not substituted for per-cell API/task/Worker operation checks. A populated old-task upgrade, cross-version effect checks, task retry/DLQ/invalid-payload operation matrix and post-upgrade Worker-drain drill remain outstanding.

## Safe operating and recovery boundary

### ADR-W4-012 — keep fail-closed, do not manufacture downgrade support

- **Decision:** keep the migration-history and exact-readiness guards; keep Wave 3 downgrade blocked after revision 012. Do not edit migration 012, drop its table, alter migration history, or broaden the accepted schema range without compatibility evidence.
- **Alternatives rejected:** deleting the newer-schema check would allow an untested binary to run; dropping the additive table would discard limiter state and still would not repair the history/readiness mismatch; rollback SQL is not a substitute for an application compatibility window.
- **Consequence:** recovery is roll-forward with a Wave 4-compatible binary, or restore a complete pre-012 database plus matching artifact state to a separate target and explicitly cut over. The latter has not been rehearsed for production.
- **Follow-up:** before future DDL, build and deploy a pre-migration compatibility bridge from the old schema, define supported schema ranges and migration ownership, test mixed-version API/Worker operation, then apply expand/contract DDL. A specific RPO/RTO cannot be promised until target infrastructure and restore timings are approved and measured.

Until a compatible release procedure is implemented and rehearsed:

1. Treat applying migration 012 as an irreversible deployment boundary for Wave 3 binaries.
2. Prefer a forward fix using a Wave 4-compatible binary. Preserve database and artifact-store snapshots before any release migration.
3. If binary downgrade is unavoidable, restore a complete, verified pre-012 database and matching artifact state to a separate target, validate it with the matching binary, then perform an explicitly approved cutover. A partial table drop or edited migration history is not a restore plan.
4. Before a future migration, deliver a pre-migration compatibility release that has an explicit supported-schema range, does not auto-apply the new DDL during the compatibility rollout, and is tested in mixed-version API/Worker combinations. Only then stage the additive migration and validate expand/contract behavior.

The current branch contains no such pre-012 compatibility bridge, production snapshot rehearsal, target-platform rollout, or approved cutover. Package F remains `PARTIAL`; the operational matrix is also limited to startup/readiness plus separate suite evidence. Overall status remains `WAVE4_ENGINEERING_PARTIAL`; production remains `PRODUCTION_NOT_APPROVED`.
