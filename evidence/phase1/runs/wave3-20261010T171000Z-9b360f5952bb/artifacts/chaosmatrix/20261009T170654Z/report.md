# Chaos matrix: 20261009T170654Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 7.76 / 10.36 / 11.58 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 7.17 / 8.72 / 9.58 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 47.76 / 57.90 / 60.07 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 42.86 / 47.12 / 50.84 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 28.19 / 31.65 / 31.88 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 48.28 / 60.52 / 61.22 ms.
