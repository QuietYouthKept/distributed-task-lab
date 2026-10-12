# Wave 4 Package E — Data Lifecycle & PostgreSQL Performance

## Result

`PARTIAL`. A conservative, read-only retention inventory and a deeper isolated pagination benchmark are implemented and tested. There is deliberately no purge/delete path: retention duration, archive and artifact reachability policy are not approved, and task deletion cascades transition events.

## Lifecycle audit and dry-run

Schema findings:

- `tasks` contains live and terminal state; task events reference it with `ON DELETE CASCADE` (`migrations/002_create_task_events.sql`). These events are transition history, not an immutable archive.
- `effects.task_id` uses `ON DELETE RESTRICT` (`migrations/009_create_effects.sql`) so durable external-effect evidence blocks task deletion.
- `security_audit_events` is append-only and carries `resource_type/resource_id`, but has no task FK. The inventory excludes tasks with a `resource_type='task'` audit reference.
- Artifact references are not normalized into a reachability table; payload/result JSON can contain URIs, and external objects may remain after a task row is deleted.
- Workers operate on pending/running rows; only terminal status rows are candidates. No batch deletion or live-object cleanup was introduced.

`internal/storage/postgres/retention_dry_run.go` runs a read-only transaction, validates the explicit cutoff against PostgreSQL `statement_timestamp()`, imposes a 5-second statement timeout, and rejects limits outside 1–1000. It lists only old terminal tasks whose `result` is null and which have no effects or security-audit reference. `cmd/retention-dry-run` additionally requires `RETENTION_DRY_RUN=true`, a database URL, and an explicit RFC3339 cutoff. The command cannot delete.

On isolated PostgreSQL 16.15, `go test -race ./internal/storage/postgres -run '^TestRetentionDryRun' -count=1 -v` passed at implementation commit `983072bc8048d5cbc74beee21e94344d5fb0e648`. The test confirmed only the conservative task appeared and event count did not change; it also covers invalid cutoff/limit. Evidence: `E:\\distributed-task-lab-temp\\wave4-artifacts\\package-e-retention-race-983072b-20261010`. A first attempt's baseline assertion failed because `Enqueue` had already emitted an event; that failure is retained separately under `package-e-retention-targeted-20261010` and is not counted as passing.

## Pagination experiment

Clean source commit: `cc1d9ccc88db0da9c0420d15927b557cdd8765b9`; tree `b5bd15883903e4ed39ca38a8e1a8042ea6e4f03f`. Run `20261010T082152Z-afccda67`; Go 1.26.9; Windows AMD64; isolated Docker PostgreSQL 16.15; 128 MB `shared_buffers`, 4 MB `work_mem`; 100,000 rows; page size 500; seed `distributed-task-lab-pagination-v2-skewed-seed-20261010`; 40 samples/query. The generator includes 70% hot-owner, 1% rare-owner and 29% spread across 20 owners.

The keyset walk returned all 100,000 initial rows in 201 pages with zero duplicates. A concurrent insert test wrote 50 rows after the first page; continuation still had zero duplicates and saw all initial rows. API-list latency on this host:

| Query | P50 ms | P95 ms | P99 ms |
|---|---:|---:|---:|
| Head | 4.970 | 6.848 | 6.972 |
| Middle keyset | 4.529 | 5.510 | 5.524 |
| Tail keyset | 1.642 | 2.050 | 2.093 |
| Broad owner (70%) | 4.589 | 5.964 | 5.993 |
| Broad-owner deep keyset | 4.847 | 5.447 | 5.839 |
| Rare owner (1%) | 4.810 | 5.405 | 5.593 |
| Rare-owner deep keyset | 5.396 | 5.878 | 5.902 |

The EXPLAIN selects the full task row used by `PostgresQueue.ListTasks`, not just `id`. At the sampled head, broad-owner plans used the global `idx_tasks_created_at` and incremental sort, removing 240 rows by filter to return 511 scan rows. Sparse-owner head used `idx_tasks_creator_created_at_id`; sparse-owner deep keyset used a bitmap heap/index scan followed by a sort. Sampled plans were warm-cache (zero shared read blocks for the listed owner head plans); this does not characterize cold-cache production behavior.

At 100K rows, task indexes measured approximately 9.17 MiB for `idx_tasks_created_at_id`, 11.31 MiB for `idx_tasks_creator_created_at_id`, and 11.31 MiB for `idx_tasks_owner_created`. The latter is non-partial while the former is partial; they overlap for non-null owners but are not proven interchangeable for all historical/null-owner semantics. Do not remove either without compatibility and workload review.

Same-run write experiment used temporary tables, five repetitions of 5,000 identical `INSERT SELECT` rows, and the same primary key in every case. P50 was 9.204 ms with no pagination indexes, 18.198 ms with global plus partial-owner indexes (+97.7%), and 22.035 ms with one additional overlapping non-partial owner index (+21.1% incremental). Relation sizes were 819,200, 1,703,936, and 2,203,648 bytes respectively. These are one host/container sample, not an SLO or direct production index removal evidence.

Raw manifest, report, command log and SHA256: `E:\\distributed-task-lab-temp\\wave4-artifacts\\package-e-pagination-final-cc1d9cc-20261010`.

## Remaining gaps

- No retention duration, archive-before-delete policy, backup/restore guarantee, task-event retention requirement, or purge authorization exists.
- Object references are not tracked in a normalized reference table, so dry-run output is not a safe-to-delete guarantee.
- CLI execution against an operator database is not verified; only its build/guard and underlying PostgreSQL inventory function are covered locally.
- No index was removed or added. Benchmark values need repeated runs under controlled DB/cache/CPU conditions before a schema change; hosted, staging, and production results are not available.
- `PRODUCTION_NOT_APPROVED` remains in force.
