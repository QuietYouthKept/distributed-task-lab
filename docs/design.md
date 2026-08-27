# Design decisions

This compatibility entrypoint points to the maintained documents:

- [Architecture](architecture.md) for authority and lock order.
- [Guarantees](guarantees.md) for precise guarantees/non-guarantees.
- [Failure model](failure-model.md) for crash, lease, clock, and shutdown behavior.
- [Handler idempotency](handler-idempotency.md) for effect semantics.
- [Migrations](migrations.md) for forward-only schema policy.

The central design choice is at-least-once execution with database fencing,
not exactly-once effects. PostgreSQL remains the sole durable coordination
service; queue rows serialize aggregate admission, task rows serialize
ownership, and polling remains the correctness fallback.
