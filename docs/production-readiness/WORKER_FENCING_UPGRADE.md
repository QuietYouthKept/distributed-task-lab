# Worker database fencing upgrade

Migration 013 replaces the Worker's direct task-table writes with database
transition routines bound to PostgreSQL `session_user`. Migration 012 is
unchanged. The routines cover claim/reclaim, lease renewal, ACK, retry/DLQ,
expired final-attempt terminalization, and identity-stamped lifecycle events.
Worker logins have SELECT-only access to `tasks`; they cannot directly update
task state or insert lifecycle events.

## Compatibility boundary

The permission change is intentionally incompatible with old Worker binaries.
They use direct `UPDATE tasks` and must not remain connected after the runtime
grant change. The selected rollout is a coordinated maintenance window:

1. Put the API into maintenance/read-only mode and stop accepting new work.
2. Drain every old Worker, verify no Worker-owned task remains actively
   executing, then stop all old Worker processes and revoke/rotate their shared
   database credential. Do not apply the grant change while old Workers run.
3. Take and verify a PostgreSQL backup. Record the current source/image digests
   and database migration version.
4. Deploy the new API/Worker-compatible binaries with API and Migrator URLs kept
   separate. Apply forward migration 013 using the Migrator identity.
5. Use `deployments/local-first/postgres/upgrade-worker-login.sh` once as the
   database administrator to convert the legacy shared `tasklab_worker` login
   into a NOLOGIN group and create the isolated Worker A login. With the
   Compose project and secrets already selected, the invocation is:
   `docker compose exec -e PGPASSWORD_FILE=/run/secrets/postgres_superuser_password -e TASKLAB_WORKER_A_PASSWORD_FILE=/run/secrets/tasklab_worker_a_password postgres sh /opt/tasklab/upgrade-worker-login.sh`.
   For every
   additional Worker, create a separate non-superuser LOGIN and grant membership
   in the `tasklab_worker` NOLOGIN group. Insert exactly one mapping into
   `worker_db_identities(login_role, worker_id)`. Never reuse a login across
   Worker instances. The login name must satisfy PostgreSQL's 63-byte role
   name limit and the migration's conservative identifier check.
6. Apply `deployments/local-first/postgres/runtime-grants.sql` with the
   Migrator role and a trusted `TASKLAB_WORKER_ID` for Worker A. For additional
   instances, provision their logins and identity mappings explicitly before
   starting them. Keep passwords in the deployment secret store only.
7. Start one new Worker and verify its readiness reports schema 13, its
   database identity matches `WORKER_ID`, and claim/renew/ACK plus direct-SQL
   denial tests pass. Then start the remaining Workers and re-enable API writes.

The repository's local-first bootstrap provisions `tasklab_worker_a` for its
single Worker. Multi-host Worker B needs its own login and mapping; do not point
it at Worker A's URL. The `compose.worker-b.yml` secret path is intentionally
separate.

## Rollback and recovery

Migration 013 is forward-only; do not edit or replay Migration 012 and do not
restore the old broad Worker UPDATE grant as an operational rollback. If the
new Worker fails after the grant cutover, keep task submission paused, repair
or roll forward the Worker, or restore the verified pre-upgrade database
backup to a separate recovery target and reconcile any external effects before
cutover. The old binary cannot be safely restarted against the tightened
grants. A backup restore loses writes after its snapshot, so enumerate and
reconcile them before traffic resumes.

This is a rollout procedure, not evidence of production approval. It must be
exercised against an isolated PostgreSQL target and reviewed for the specific
deployment before any production use.
