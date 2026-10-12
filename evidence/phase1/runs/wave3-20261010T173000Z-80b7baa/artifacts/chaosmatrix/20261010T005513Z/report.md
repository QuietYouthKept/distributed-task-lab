# Chaos matrix: 20261010T005513Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 6.65 / 8.04 / 8.41 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 6.23 / 7.41 / 7.75 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 56.94 / 75.97 / 79.75 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 56.87 / 62.47 / 64.79 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 40.12 / 54.29 / 56.30 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 67.93 / 83.07 / 93.82 ms.
