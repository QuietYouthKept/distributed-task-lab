# Chaos matrix: 20261009T122538Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `postgres_short_disconnect`: 28/30 passed; recovery p50/p95/p99 40.61 / 46.93 / 47.35 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 41.52 / 48.08 / 48.51 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 15.11 / 17.22 / 19.52 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 43.76 / 49.24 / 50.05 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 4.16 / 5.46 / 7.00 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.52 / 6.59 / 7.19 ms.
