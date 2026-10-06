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

Set separate high-entropy `API_TOKEN` and `ADMIN_TOKEN`, terminate TLS at the
ingress, restrict PostgreSQL network access, and inject secrets through the
platform secret store. Empty tokens are local-development mode only.

Set `APP_PROFILE=production` for deployed processes. Startup validates all
environment parsing and cross-field limits before opening the database. The
production profile requires separate API/admin tokens, PostgreSQL TLS, a
non-default database password, and an `s3://` or `minio://` artifact location;
the log includes only a redacted database endpoint and a configuration checksum.

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

### Connection exhaustion and worker drain

Lower worker replica/concurrency or claim batch size before raising the
database pool limit. Preserve headroom for API and migration connections. For a
safe rollout, remove a worker from traffic, send `SIGTERM`, wait at least
`SHUTDOWN_TIMEOUT`, and then replace it; the worker stops claims while renewing
and draining in-flight work.
