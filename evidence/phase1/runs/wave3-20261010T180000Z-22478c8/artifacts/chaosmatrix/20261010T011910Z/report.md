# Chaos matrix: 20261010T011910Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 6.57 / 8.21 / 8.36 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 6.35 / 7.93 / 8.07 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 44.64 / 47.16 / 47.22 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 43.69 / 52.32 / 52.95 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 24.94 / 26.65 / 27.15 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 47.13 / 52.16 / 52.94 ms.
