# Chaos matrix: 20261009T120032Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 39.69 / 42.99 / 43.68 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 14.36 / 15.73 / 15.93 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 43.76 / 49.07 / 49.68 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 4.47 / 5.56 / 5.97 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.37 / 5.93 / 6.17 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 40.10 / 46.36 / 65.56 ms.
