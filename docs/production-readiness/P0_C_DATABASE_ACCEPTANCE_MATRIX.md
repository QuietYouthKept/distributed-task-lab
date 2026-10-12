# PostgreSQL privilege and migration acceptance matrix

| Check | Current source/test | Required assertion | This checkpoint |
|---|---|---|---|
| Fresh migration | `TestMigrationsFreshAndRerun` | Schema reaches latest migration; rerun is idempotent | NOT RUN (no database supplied) |
| Upgrade / history validation | `migration_integration_test.go` | Gaps/checksum mismatch fail closed | NOT RUN |
| API lifecycle | `TestRuntimeRoleLeastPrivilege` | enqueue, pause/resume and audit insert | Existing integration test; not run |
| Worker claim + ACK | same | restricted runtime can claim and fenced ACK | Existing integration test; not run |
| Worker renewal/retry/DLQ | queue repository integration tests | required DML succeeds under exact worker grants | NOT VERIFIED under local-first grants |
| API/Worker DDL denied | runtime-role integration | CREATE/ALTER/role denied | Existing integration assertions; not run |
| DELETE/TRUNCATE denied | runtime-role integration | destructive task/audit actions denied | Existing integration assertions; not run |
| Event/audit immutability | runtime-role integration | UPDATE/DELETE denied | Existing integration assertions; not run |
| Unintended table/column write denied | new privilege fixture needed | unrelated table/column write denied | PARTIAL; broad table-level UPDATE remains |
| Migrator can migrate; runtime cannot | integration acceptance needed | migrator DDL succeeds, API/Worker DDL fails | PARTIAL; scripts configured, exact local roles not run |
| Sequence access | checked-in runtime grant SQL | event/audit insert sequences usable, others denied | Static only |
| Role attributes | bootstrap SQL + integration | all runtime roles non-superuser/no create-role/db | Static + pre-existing generated-role test |

The two PowerShell database scripts require an explicit loopback URL to a
disposable maintenance database with the `tasklab_local_test_admin` name. They
set `REQUIRE_TEST_DATABASE=1`, so missing PostgreSQL fails instead of making
integration tests appear to pass via `Skip`.
