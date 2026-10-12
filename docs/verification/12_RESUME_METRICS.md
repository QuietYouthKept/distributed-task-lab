# Resume Metrics and Safe Claims

Only the entries explicitly marked `HISTORICAL_MEASURED` have numeric test values. They are tied to old source snapshots and should be used only with that qualifier.

| Claim / metric | Status | Safe wording |
|---|---|---|
| PostgreSQL-backed historical worker matrix | HISTORICAL_MEASURED | “Built an in-process Go worker/queue benchmark against PostgreSQL and recorded a 1,000-task backlog-drain matrix from 1 to 16 logical worker pools; the historical run completed 1,000/1,000 tasks without DLQ or duplicate claims.” Include source date/commit and do not call this 16 independent processes. |
| 593 tasks/s claim | NOT_VERIFIED | Do not use; original raw timing/samples were not found. |
| Azure LLM trace audit | CURRENT_MEASURED (dataset audit only) | “Audited local Azure LLM Code/Conversation traces: 8,819 and 19,366 timestamped records; documented token and arrival distributions.” Not an execution or model-quality result. |
| Replay generation | CURRENT_MEASURED (generation only) | “Implemented deterministic, bounded conversion and generated 1,000 replay workload records; generator tests passed 7/7.” State `GENERATED_NOT_EXECUTED`. |
| Current API TPS / E2E p95/p99 | NOT_VERIFIED | No claim. |
| Maximum current independent processes/hosts | NOT_VERIFIED | No claim; retained historical worker pools were in-process. |
| Crash recovery percentile, long soak, peak memory, restore RTO | NOT_VERIFIED | No claim. |

Resume-safe versions:

**Currently defensible:** “Implemented and tested a bounded deterministic workload replay converter for public Azure LLM traces, including source/output hashing and no-clobber publication. Audited trace quality and preserved data provenance. Separately, a historical PostgreSQL worker-pool benchmark ran 1,000-task backlog-drain cases at 1/4/8/16 in-process workers; those figures are not current-release or full API E2E capacity.”

**After cloud acceptance (template only):** “On `<instance type / region / image digest>`, executed `<tasks>` through API→PostgreSQL→Worker→Gateway→OSS; measured `<successful TPS>`, E2E p95/p99 `<...>`, crash recovery `<...>`, and independent restore `<...>` under `<run ID>`, source `<SHA>`. Replace every placeholder only from sealed evidence.”
