# Chaos matrix: 20261009T094517Z

Each listed trial is database-oracled and recorded in `events.jsonl`. Recovery includes fault injection and convergence time; compare p95 to the explicit deployment budget (lease timeout + poll interval + handler startup).

- `worker_sigkill`: 30/30 passed; recovery p50/p95/p99 41.16 / 44.29 / 44.66 ms.
- `sigterm_drain`: 30/30 passed; recovery p50/p95/p99 16.24 / 18.07 / 18.37 ms.
- `stale_generation`: 30/30 passed; recovery p50/p95/p99 47.32 / 50.64 / 51.38 ms.
- `effect_before_ack`: 30/30 passed; recovery p50/p95/p99 5.12 / 6.67 / 6.81 ms.
- `provider_response_lost`: 30/30 passed; recovery p50/p95/p99 4.60 / 6.26 / 6.73 ms.
- `postgres_short_disconnect`: 30/30 passed; recovery p50/p95/p99 41.62 / 51.42 / 51.88 ms.
