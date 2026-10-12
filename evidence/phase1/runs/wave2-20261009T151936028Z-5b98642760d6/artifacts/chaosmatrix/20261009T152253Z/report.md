# Chaos matrix: 20261009T152253Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.34 / 6.27 / 6.98 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.26 / 5.80 / 5.95 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 39.99 / 45.92 / 47.14 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.07 / 45.95 / 46.29 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 24.53 / 28.21 / 28.34 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 42.01 / 45.50 / 45.84 ms.
