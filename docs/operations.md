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

## Alerts

Alert on readiness failures, oldest pending age, DLQ/retry/reclaim growth,
renew/fencing failures, worker heartbeat age, DB pool waits, and sustained
claimed-not-started depth. Back up PostgreSQL and rehearse restore procedures.
