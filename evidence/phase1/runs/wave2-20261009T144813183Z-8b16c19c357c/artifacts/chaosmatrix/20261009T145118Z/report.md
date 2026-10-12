# Chaos matrix: 20261009T145118Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `stale_generation`: 30/30 passed; recovery p50/p95/p99 41.39 / 45.11 / 46.96 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.29 / 5.93 / 6.41 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.06 / 5.86 / 5.98 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 39.68 / 49.27 / 52.48 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 37.76 / 39.99 / 40.22 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 21.51 / 23.23 / 23.46 ms.
