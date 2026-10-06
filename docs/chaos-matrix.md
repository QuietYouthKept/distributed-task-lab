# Repeated chaos matrix

`make chaos-matrix` provisions an isolated PostgreSQL database and repeats the
crash, lease, fencing, effect, and reconnect scenarios (`CHAOS_REPETITIONS=30`
by default). It writes `benchmarks/chaos/<run-id>/manifest.json`,
`events.jsonl`, `summary.json`, and `report.md`; output directories are never
overwritten.

The matrix asserts stale success/failure/renew writes are rejected, receipt
reconciliation yields one durable business effect despite a lost response, and
crash/drain/disconnect trials converge to a terminal result. Compare observed
p95 recovery to lease timeout + poll interval + handler startup.
