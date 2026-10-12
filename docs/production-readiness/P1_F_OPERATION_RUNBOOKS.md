# P1-F — Local operations runbooks

All commands below are diagnostic starting points. Never expose secret values,
delete data, disable authorization, or clean resources not owned by this run.

| Incident | Trigger / first checks | Safe recovery | Escalate when / prohibited |
|---|---|---|---|
| API down | readiness, container logs, resource snapshot | restart only exact run's API after dependency health | DB/migration uncertain; do not bypass auth |
| Worker down | worker health, lease events, recent attempt/version | start one owned worker and observe lease reclaim | stale ACK or repeated effect uncertainty; do not reset lease rows manually |
| PostgreSQL unavailable | healthcheck, connection pool metrics, container logs | restore connectivity; preserve DB volume | corruption or migration mismatch; never reinitialize source volume |
| MinIO unavailable | health endpoint, bucket policy, disk | restore service with same isolated data volume | unknown digest/data loss; do not substitute another store silently |
| Gateway timeout | request ID/fingerprint, task attempt, effect ledger | reconcile unknown result before retry | possible external execution; do not mark failure as definitely not executed |
| Queue backlog | queue depth/oldest age/worker availability | scale only within resource budget; inspect paused/rate-limited queues | backlog from repeated failure or saturation |
| DLQ surge | failed attempt/error categories | preserve payload reference and inspect bounded sample | suspected poison/security payload; no bulk requeue |
| Lease renewal failure | lease version, worker heartbeat, DB latency | allow normal bounded reclaim and inspect fencing | stale ACK unexpectedly accepted |
| Migration failure | migrator exit/log, schema_migrations/checksum | stop rollout; preserve schema and forward-fix plan | never edit applied migration or downgrade schema blindly |
| Backup failure | dump/object inventory logs, free disk | retry into a new run directory after cause fixed | snapshot inconsistency; preserve partial artifacts for diagnosis |
| OIDC/JWKS outage | issuer health, TLS chain, cache age | restore trusted issuer/certificate; fail closed | never disable signature, issuer, or audience checks |
| Alert delivery failure | rule state, receiver health, delivery log | restore local receiver and replay only if supported | production on-call routing remains unverified |

Evidence: record exact source commit/tree, command, exit code, resource
snapshot, redacted raw log and checksum after file close. Do not put tokens,
DSNs, or private keys in issue reports.
