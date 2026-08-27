# Guarantees and non-guarantees

## Guaranteed by committed database transitions

- A task row has one current owner generation.
- Stale generations cannot renew, succeed, or fail the current generation.
- Per-queue pending admission is serialized and cannot exceed a positive
  `max_concurrency`.
- Expired-running ownership can be replaced while at capacity.
- Queue pause blocks new claims, not existing executions.
- Queue rate/burst is cluster-global claim admission.
- One non-null enqueue idempotency key produces one task and one submitted event.

## Explicit non-guarantees

- Execution and external effects are not exactly once.
- A handler that ignores context cannot be stopped in-process.
- Rate admission does not guarantee an exact downstream request completion rate.
- Strict priority does not provide starvation freedom.
- `task_events` is transition history, not tamper-proof compliance storage.

The evidence mapping is maintained in the README and `test-cards/`.
