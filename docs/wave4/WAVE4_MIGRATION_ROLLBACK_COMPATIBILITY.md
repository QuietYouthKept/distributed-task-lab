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
- **C — data semantics:** migration 012 contains no task/effect data transformation. A populated Wave 3 schema11 fixture was snapshotted, upgraded by Wave 4, and compared byte-for-byte at the selected logical-data snapshot. Existing task, event, queue, audit, owner, effect, retry, DLQ, lease and artifact references remained unchanged.
- **D — Worker execution:** the Wave 3 Worker cannot start on schema12, so old-binary claim/renew/ACK/retry/DLQ behavior on schema12 remains intentionally `BLOCKED`. The populated scenario instead verified Wave 4 operations after upgrade, plus Wave 3 operations against a separate restored schema11 database.

## Isolated process matrix

Runtime evidence is in `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave4-artifacts\migration-runtime-matrix-3fca936`; source snapshots, earlier failed attempts, logs, binaries, and checksums are retained under adjacent `migration-matrix-*` directories. The tested old source was archived from the stated Wave 3 SHA. Each case used an isolated PostgreSQL 16 container and a fresh database. Readiness means HTTP 200 for both API and Worker; the two expected Wave 3 rejections exited with code 1 and recorded the specific newer-schema guard.

| Binary | Schema 11 | Schema 12 |
|---|---|---|
| Wave 3 API | PASS; starts and ready | BLOCKED as expected; exits before serving: schema 12 newer than embedded 11 |
| Wave 3 Worker | PASS; starts and ready | BLOCKED as expected; exits before serving: schema 12 newer than embedded 11 |
| Wave 4 API | PASS; startup applies 012, then ready at 12 | PASS; ready at 12 |
| Wave 4 Worker | PASS; startup applies 012, then ready at 12 | PASS; ready at 12 |

Thus the process matrix confirms a real rollback boundary. It does not establish that old binaries can execute against schema12, and it does not justify deleting the additive table or editing `schema_migrations` to force startup. Migration 012 remains unchanged. Initial harness attempts (incorrect test URL, finalizer errors and bounded evidence-runner issues) remain preserved under E:; only the accepted run below is a passing migration/recovery result.

## Populated upgrade and isolated recovery evidence

Accepted local run: Wave 3 source `a8656a7b72d792986775d94fe007acc7771c072b`; Wave 4 harness/application source `0d3f48dc09c9b9f00d0c8c36305dd4520d6a2a8b`, tree `4da6f773abd71a17d27c2b0c1ad9a1eea813fd16`; PostgreSQL 16.15; pinned source-built MinIO identity recorded in the run manifest. Raw logs, database snapshots, binaries, the custom-format pre-migration dump, object checks, and SHA256SUMS are at `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave4-artifacts\populated-migration-20261010-07`.

- **`SCHEMA11_TO_12_POPULATED_UPGRADE_PASS`:** Wave 3 API/Worker generated and executed the supported fixture states (pending, success, retryable pending at attempt 1, DLQ, running lease, applied and unknown effects, events/audit, owner association, and a real MinIO-backed RAG result). The selected schema11 data snapshot hash was `9bcdaef9506e80fb44fe09f8e7cdb136cfc0480755d30904c7bcd2981a89c695`; the post-012 logical-data snapshot had the same hash. The snapshot intentionally omits migration history because schema12 must add migration 012.
- Wave 4 API/Worker then performed Get/List/Event reads, invalid-credential rejection, retry completion, expired-lease reclaim (`attempt=2`, `lease_version=2`), observed lease renewal, DLQ preservation, artifact reuse with SHA256 `c8d3273a6bae0389ecd01ad108db088c4491fcfa824fdd35d1c85d0d635d15c8`, and response-loss effect reconciliation from an unknown prior generation to an applied retry generation. This is a local deterministic provider fixture, not a real vendor integration. It is not a simultaneous mixed-version run: Wave 3 binaries still fail closed on schema12.
- **`ROLL_FORWARD_RECOVERY_PASS`:** the isolated Wave 4 API/Worker were stopped after upgrade and restarted with schema12-compatible binaries; readiness and a migrated task read succeeded. No production process or data was involved.
- **`ISOLATED_PRE_MIGRATION_RESTORE_PASS`:** the complete schema11 PostgreSQL custom-format dump (SHA256 `8b2754f48780bb697678f41cb9349fde8447494b924c78c710b936995b9f20e6`) was restored into a separate database and matched the pre-upgrade logical-data snapshot. The two MinIO objects referenced by the fixture were copied into a second isolated MinIO service and independently read back by SHA256. Wave 3 API/Worker then read the restored task, processed a restored pending task, reconciled the persisted unknown effect, and reused the restored RAG artifact. The test also confirms the expected limitation: all writes made only after the pre-migration snapshot are absent from that restore.
- These results are isolated engineering evidence only. There is no production backup cadence, RPO/RTO, target-platform rehearsal, approved cutover, or independent production approval.

The older startup/readiness matrix remains separately bound to `3fca936`; it is not relabeled as the populated run. Historical Wave 3 evidence is not substituted for any of these checks.

## Safe operating and recovery boundary

### ADR-W4-012 — keep fail-closed, do not manufacture downgrade support

- **Decision:** keep the migration-history and exact-readiness guards; keep Wave 3 downgrade blocked after revision 012. Do not edit migration 012, drop its table, alter migration history, or broaden the accepted schema range without compatibility evidence.
- **Alternatives rejected:** deleting the newer-schema check would allow an untested binary to run; dropping the additive table would discard limiter state and still would not repair the history/readiness mismatch; rollback SQL is not a substitute for an application compatibility window.
- **Consequence:** recovery is roll-forward with a Wave 4-compatible binary, or restore a complete pre-012 database plus matching artifact state to a separate target and explicitly cut over. The latter has not been rehearsed for production.
- **Follow-up:** before future DDL, build and deploy a pre-migration compatibility bridge from the old schema, define supported schema ranges and migration ownership, test mixed-version API/Worker operation, then apply expand/contract DDL. A specific RPO/RTO cannot be promised until target infrastructure and restore timings are approved and measured.

Even with the populated upgrade, roll-forward restart, and separate-target restore passing locally, direct Wave 3 binary rollback on schema12 remains blocked. Until a compatible release procedure is implemented and rehearsed:

1. Treat applying migration 012 as an irreversible deployment boundary for Wave 3 binaries.
2. Prefer a forward fix using a Wave 4-compatible binary. Preserve database and artifact-store snapshots before any release migration.
3. If binary downgrade is unavoidable, restore a complete, verified pre-012 database and matching artifact state to a separate target, validate it with the matching binary, then perform an explicitly approved cutover. A partial table drop or edited migration history is not a restore plan.
4. Before a future migration, deliver a pre-migration compatibility release that has an explicit supported-schema range, does not auto-apply the new DDL during the compatibility rollout, and is tested in mixed-version API/Worker combinations. Only then stage the additive migration and validate expand/contract behavior.

The current branch contains no pre-012 compatibility bridge, production snapshot rehearsal, target-platform rollout, or approved cutover. This populated run does not test Wave 3 on schema12 (that cell is expected blocked), nor a concurrent Wave 3/Wave 4 operation window. Package F remains `PARTIAL`; overall status remains `WAVE4_ENGINEERING_PARTIAL`, and production remains `PRODUCTION_NOT_APPROVED`.
