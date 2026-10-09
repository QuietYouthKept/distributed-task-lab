# Chaos matrix: 20261009T123400Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 14.97 / 15.64 / 16.00 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 43.23 / 45.52 / 45.76 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 4.57 / 5.20 / 6.33 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.29 / 4.86 / 5.20 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 39.43 / 41.15 / 41.18 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 43.32 / 48.23 / 48.45 ms.
