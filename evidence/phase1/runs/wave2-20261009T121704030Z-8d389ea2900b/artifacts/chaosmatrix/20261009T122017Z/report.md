# Chaos matrix: 20261009T122017Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 15.24 / 16.25 / 16.64 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.38 / 47.36 / 47.89 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 4.16 / 5.62 / 5.67 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.26 / 5.72 / 5.72 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 39.56 / 53.84 / 56.49 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 39.94 / 47.35 / 47.95 ms.
