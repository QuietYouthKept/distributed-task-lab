# Chaos matrix: 20261009T150442Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.93 / 5.60 / 5.62 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 40.18 / 43.12 / 45.66 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.98 / 61.88 / 64.65 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 23.22 / 24.67 / 28.86 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.88 / 51.88 / 57.39 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.24 / 6.38 / 7.04 ms.
