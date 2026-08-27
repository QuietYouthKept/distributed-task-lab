# Time Authority Audit

PostgreSQL `clock_timestamp()` is authoritative for distributed state: scheduling eligibility, lease expiry and renewal, retry base time, heartbeat staleness, rate-token refill, and event/database timestamps. The SQL paths are in `internal/storage/postgres/task_repository.go`.

Local Go time is used only for non-authoritative control: polling intervals, per-operation deadlines, graceful-shutdown budgets, latency metrics, logging, and handler-produced informational timestamps. Handler payload/result timing is not used to decide task ownership or scheduling. The Redis implementation is a legacy local adapter and is not the documented distributed authority.

Evidence: repository integration tests and `TL-CLOCK-002`, which passes a simulated client clock skew while observing a database-derived lease interval.
