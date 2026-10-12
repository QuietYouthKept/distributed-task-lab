# Chaos matrix: 20261009T131623Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.86 / 7.22 / 7.30 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 43.67 / 69.24 / 72.92 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.88 / 54.01 / 56.46 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 24.74 / 31.85 / 35.28 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.13 / 49.70 / 54.06 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.44 / 6.20 / 6.24 ms.
