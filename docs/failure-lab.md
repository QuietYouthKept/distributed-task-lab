# Failure lab

`make demo-failure-lab` provisions dedicated PostgreSQL and runs twelve JSONL
scenarios. Each line includes time, scenario, operation, worker/task identity,
lease generation, attempt, state, and oracle result.

Scenarios: concurrent no-duplicate claim, strict concurrency, lease reclaim,
same-worker stale fencing, renewal, retry/DLQ, concurrent enqueue idempotency,
effect-before-ack, pause/resume, cluster rate, graceful drain, and hard-crash
recovery. A non-zero exit means an oracle failed.

The effect-before-ack case intentionally shows two executions: a non-idempotent
counter reaches two, while a business-key set converges to one effect.
