# Benchmark evidence bundles

`make loadgen` writes one immutable evidence bundle under
`benchmarks/<UTC-run-id>/`. Set `LOADGEN_RUN_ID` to a stable descriptive name
when preparing a reviewable matrix point. Every bundle contains the source SHA,
runtime and database versions, workload parameters, raw claim samples,
PostgreSQL counters, a machine-readable summary, and a short report.

Run `make benchmark-matrix` for the fixed worker scaling curve (1, 4, 8, 16
workers; four queues; 1 KiB payload; backlog drain). It deliberately does not
commit a fabricated throughput number: commit the generated directory from the
machine whose CPU, storage, PostgreSQL settings, and Docker limits are recorded
in `manifest.json`.
