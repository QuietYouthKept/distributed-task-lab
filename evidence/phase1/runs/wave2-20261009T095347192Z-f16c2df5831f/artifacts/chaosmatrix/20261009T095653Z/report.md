# Chaos matrix: 20261009T095653Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.37 / 6.47 / 6.59 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.18 / 6.50 / 6.55 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 43.64 / 46.82 / 48.11 ms.
- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 43.21 / 48.09 / 51.54 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 18.04 / 21.00 / 21.94 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 44.59 / 50.35 / 50.43 ms.
