# Chaos matrix: 20261009T151548Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 43.06 / 44.27 / 44.27 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.44 / 42.74 / 43.56 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 25.04 / 30.74 / 34.14 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 45.25 / 54.11 / 54.78 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 7.06 / 8.48 / 9.08 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 6.55 / 7.94 / 8.27 ms.
