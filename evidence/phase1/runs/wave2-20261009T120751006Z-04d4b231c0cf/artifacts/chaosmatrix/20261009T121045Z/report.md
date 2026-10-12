# Chaos matrix: 20261009T121045Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 41.45 / 47.87 / 48.39 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.76 / 42.98 / 46.21 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 15.10 / 16.73 / 18.10 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.96 / 55.85 / 56.27 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 6.59 / 9.65 / 9.71 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.11 / 6.86 / 7.15 ms.
