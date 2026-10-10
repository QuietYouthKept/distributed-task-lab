# Chaos matrix: 20261010T024052Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 40.47 / 50.87 / 53.76 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 26.16 / 29.98 / 32.31 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 47.24 / 68.67 / 70.27 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 6.25 / 7.65 / 7.85 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 5.75 / 6.75 / 7.23 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 47.24 / 66.48 / 68.39 ms.
