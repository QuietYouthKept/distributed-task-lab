# Chaos matrix: 20261009T135331Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `stale_generation`: 30/30 passed; recovery p50/p95/p99 43.59 / 55.75 / 57.01 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.76 / 10.94 / 11.72 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 8.67 / 10.79 / 11.30 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 42.51 / 55.25 / 58.60 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 39.81 / 46.66 / 48.38 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 23.86 / 28.24 / 28.81 ms.
