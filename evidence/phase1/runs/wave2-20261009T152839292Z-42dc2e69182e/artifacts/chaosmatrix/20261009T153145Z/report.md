# Chaos matrix: 20261009T153145Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 22.70 / 24.57 / 25.30 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.49 / 55.90 / 62.33 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.83 / 6.86 / 6.91 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.29 / 6.26 / 6.46 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 41.92 / 54.64 / 56.53 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 39.43 / 131.18 / 141.36 ms.
