# Chaos matrix: 20261009T141633Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 37.28 / 39.19 / 40.53 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 22.68 / 32.83 / 35.17 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 41.93 / 44.93 / 45.56 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.03 / 6.51 / 6.57 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.84 / 5.90 / 5.93 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 38.65 / 40.51 / 41.18 ms.
