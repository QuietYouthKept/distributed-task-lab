# Chaos matrix: 20261009T133306Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.48 / 7.41 / 7.61 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 43.88 / 52.50 / 53.18 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.95 / 51.95 / 52.56 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 25.36 / 29.77 / 34.64 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 45.24 / 58.32 / 59.25 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.71 / 6.56 / 6.95 ms.
