# Performance Evidence Matrix

| Metric | Value | Classification | Source/runtime and boundary |
|---|---:|---|---|
| Maximum worker count in retained matrix | 16 in-process Go worker pools (concurrency 1 each) | HISTORICAL_MEASURED | SHA `cd3696e7141527b01b2b0361f2674729855203d1`; one PostgreSQL-backed Windows run. Not 16 OS processes or hosts. |
| Maximum task count in retained matrix | 1,000 tasks per run | HISTORICAL_MEASURED | Four runs, 1/4/8/16 workers, four queues, 1 KiB payload. |
| Scheduler backlog drain throughput | 39.8119 / 80.2218 / 160.3302 / 159.0648 tasks/s at 1/4/8/16 workers | HISTORICAL_MEASURED | Legacy denominator is configured task count divided by post-submit drain time; do not compare as full-run throughput. Exact raw hashes in `03_HISTORICAL_EVIDENCE_AUDIT.md`. |
| Successful tasks / DLQ | 1,000 / 0 each run | HISTORICAL_MEASURED | Direct DB queue + in-process worker, noop/mock handler. |
| E2E task latency p95 | 21.8667 / 11.1157 / 6.2250 / 6.3058 seconds | HISTORICAL_MEASURED | DB created-to-finished samples from historical run; not API or provider latency. |
| Current-source API RPS / true E2E TPS | — | NOT_VERIFIED | No current SHA API→Worker→Gateway→Artifact run. |
| Current-source E2E p95/p99 | — | NOT_VERIFIED | Existing historical samples do not bind to current code. |
| Crash recovery p95 / soak duration / peak memory | — | NOT_VERIFIED | No current measurable recovery/soak run. |
| Current Azure LLM replay size | 1,000 generated JSONL rows | PREPARED_NOT_RUN | Dataset conversion only; 0 tasks submitted. Do not state as scheduler throughput/load result. |

Resume-safe wording is limited to the explicit historical scope; no current throughput or production-capacity claim is available.
