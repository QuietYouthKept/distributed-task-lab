# Architecture

PostgreSQL owns all durable scheduling and ownership state. API instances only
enqueue/query; worker processes may disappear at any point.

## Authorities

- `tasks` row: task state and current fencing generation.
- `queues` row: pause, strict concurrency, and shared token-bucket admission.
- PostgreSQL `clock_timestamp()`: due time, expiry, renewal, retry base, and
  heartbeat freshness.
- `schema_migrations`: ordered forward-only schema version and checksum.

Claim transactions lock eligible tasks with `FOR UPDATE SKIP LOCKED`, derive the
distinct queue set, then lock queue rows by ascending name. Pending admission
consumes capacity; expired-running reclaim does not. Rate tokens are refilled and
consumed under the same queue lock. State transition and `task_events` insertion
commit atomically.

Workers bound locally owned work to `concurrency + min(batch, concurrency)`.
Renewal starts immediately after claim, including local queue wait. Completion
uses the full `(task_id, worker_id, lease_version)` credential.

Polling is the correctness path. A future LISTEN/NOTIFY optimization may reduce
latency, but notifications must never become durable authority.
