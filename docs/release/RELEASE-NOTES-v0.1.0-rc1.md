# v0.1.0-rc1 — correctness-first PostgreSQL task lab

This release candidate packages the PostgreSQL task-execution laboratory for
review and interview demonstration.

Highlights:

- strict queue capacity and shared rate admission using PostgreSQL authorities;
- leases, generation fencing, renewal, crash recovery, and graceful drain;
- checksummed/serialized migrations and DB-clock scheduling;
- authenticated API/admin surfaces, readiness, traces, and bounded metrics;
- reproducible Asset Lab and Docker fault evidence with an independent checker;
- non-root API and worker runtime images.

Known boundaries: execution is at-least-once; handlers must make external
effects idempotent. This release has no PostgreSQL HA, long-soak, disk-loss, or
multi-host network-partition guarantee.
