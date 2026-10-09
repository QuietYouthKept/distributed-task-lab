# Chaos matrix: 20261009T132557Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 6.06 / 7.50 / 7.55 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.50 / 6.38 / 6.40 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 43.23 / 53.49 / 68.26 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 42.99 / 50.87 / 56.55 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 24.01 / 29.11 / 29.56 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.38 / 55.13 / 56.02 ms.
