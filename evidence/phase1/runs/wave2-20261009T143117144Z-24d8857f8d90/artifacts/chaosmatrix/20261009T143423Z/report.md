# Chaos matrix: 20261009T143423Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 6.38 / 8.26 / 9.30 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 44.37 / 55.41 / 56.65 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 42.92 / 53.16 / 55.59 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 24.45 / 26.82 / 26.99 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 45.73 / 53.04 / 54.28 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 7.25 / 9.10 / 9.72 ms.
