# Chaos matrix: 20261009T172000Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 45.32 / 50.51 / 51.11 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 26.40 / 29.62 / 30.76 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 47.74 / 54.13 / 55.43 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.75 / 7.61 / 11.50 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 7.52 / 10.33 / 14.70 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 48.09 / 53.87 / 54.02 ms.
