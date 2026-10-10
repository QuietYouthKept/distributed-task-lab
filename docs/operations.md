# Operations

## Health

- API `/live`: process loop is alive.
- API `/ready`: database reachable and exact embedded schema installed.
- Worker `:9091/live`, `/ready`, `/metrics`: process, database/schema, runtime.

Remove an instance from traffic on readiness failure. Do not restart-loop a
migration checksum mismatch; restore the original migration or add a new one.

## Shutdown

Send `SIGTERM`, allow at least `SHUTDOWN_TIMEOUT`, then use the platform hard
kill. The worker stops claims first and continues renewal while draining.

## Security

Set `APP_PROFILE=production` only after the deployment supplies an HTTPS OIDC
issuer, audience, JWKS endpoint, and explicit role mappings. Production rejects
`API_TOKEN`/`ADMIN_TOKEN`; these are development compatibility only. Provide
`API_CURSOR_SIGNING_KEY` with at least 32 random bytes through the platform
secret store. Rotating this key invalidates outstanding pagination cursors.

Production API startup also validates a non-local PostgreSQL URL with TLS and
non-default credentials, an HTTPS S3-compatible endpoint, bucket and explicit
credentials, and the HTTPS evaluation gateway configuration. The API logs a
redacted DB endpoint plus a checksum that excludes credentials. Do not place
secret values in logs, task payloads, shell history, or evidence bundles.

The worker currently fails closed in production until shared artifact storage
is wired through every artifact-producing handler and verified against an
actual MinIO/S3 service. A configured S3 endpoint alone is not proof that
workers share artifacts. The production task execution allowlist is currently
empty; structurally valid Mock/deterministic tasks are intentionally rejected.
Do not weaken either gate to make a demo run.

Queue governance rate limits apply cluster-wide through PostgreSQL. The API
ingress token bucket is per process only and is an overload guard, not a
cluster-wide tenant quota.

## Backup and restore

Use the platform's encrypted backup system for PostgreSQL and the configured
object store. Treat task payloads/results, task events, security audit rows,
effect ledger records, and referenced artifacts as one recovery set. Object
retention must not delete an artifact still referenced by a live task or an
audit/effect record. Never use `TRUNCATE` or delete audit/effect history as a
routine cleanup mechanism.

For a PostgreSQL logical recovery, use an isolated target database and a
verified backup, then:

```sh
pg_restore --exit-on-error --no-owner --dbname="$RESTORE_DATABASE_URL" backup.dump
```

After restore, confirm `/ready` reports the expected schema, compare task,
event, audit and effect counts with the backup manifest, verify representative
artifact hashes against the object store, and exercise one non-production
claim/complete workflow before switching traffic. The repository does not yet
contain a rehearsed production restore or RPO/RTO evidence; this procedure is a
runbook, not a claim that disaster recovery has been tested.

## Alerts

Alert on readiness failures, oldest pending age, DLQ/retry/reclaim growth,
renew/fencing failures, worker heartbeat age, DB pool waits, and sustained
claimed-not-started depth. Back up PostgreSQL and rehearse restore procedures.

## Incident runbook

### Queue backlog

Check per-queue depth, oldest pending age, worker readiness, handler failure
rate, rate configuration, and `tasklab_db_pool_wait_seconds_total`. If a queue
is rate- or concurrency-limited, raise only the proven bottleneck and first
confirm downstream capacity. A queue authority lock is expected for strict
admission; rising wait with flat handler throughput points to PostgreSQL or
queue-authority contention.

### Lease expiry or fencing surge

Compare worker heartbeat age with handler duration and `LEASE_TIMEOUT`. Do not
turn off fencing or force an old worker to acknowledge: a stale generation is
supposed to be rejected. Drain unhealthy workers, let a new worker reclaim the
lease, and inspect the task event history. Repeated expiry means the lease is
too short for real work, renewal is failing, or workers are overloaded.

### DLQ and replay

Filter `/dlq`, inspect the payload, error, attempts, and event sequence, then
repair the payload or downstream dependency. Requeue only after identifying the
failure class; `POST /tasks/{id}/requeue` resets it to pending and creates an
audit event. For external effects, reconcile the provider receipt first rather
than blindly replaying.

### Unknown external effect

Find the `effects` record using the task and business operation key. Its
generation and request hash identify the actual provider request. Query the
provider with that idempotency key before changing task state. If the provider
cannot be queried or made idempotent, set the effect to `manual_resolution` and
resolve it with a documented operator decision; do not retry automatically.
The API exposes unresolved records at `GET /v1/effects` to operators. Only an
admin may call `POST /v1/effects/{id}/resolve`, and the required reason is
written to the append-only security audit in the same transaction. This
manual state means “operator reviewed”; it does not prove provider settlement.

### Connection exhaustion and worker drain

Lower worker replica/concurrency or claim batch size before raising the
database pool limit. Preserve headroom for API and migration connections. For a
safe rollout, remove a worker from traffic, send `SIGTERM`, wait at least
`SHUTDOWN_TIMEOUT`, and then replace it; the worker stops claims while renewing
and draining in-flight work.
