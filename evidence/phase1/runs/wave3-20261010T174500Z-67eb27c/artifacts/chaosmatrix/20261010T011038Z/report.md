# Chaos matrix: 20261010T011038Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 42.20 / 46.03 / 47.01 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 41.39 / 45.77 / 50.40 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 26.75 / 29.53 / 31.96 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 45.86 / 52.40 / 55.33 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.71 / 7.38 / 7.73 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.48 / 6.99 / 7.08 ms.
