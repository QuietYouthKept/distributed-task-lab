# Chaos matrix: 20261009T153928Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.13 / 57.53 / 70.47 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.33 / 6.40 / 6.51 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.80 / 8.20 / 8.71 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 42.19 / 51.57 / 54.00 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 41.15 / 45.78 / 48.72 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 24.43 / 30.57 / 31.07 ms.
