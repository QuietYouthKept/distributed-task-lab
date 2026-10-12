# Engineering interview guide — distributed-task-lab

Use these as accurate talking points, not claims of production operation. The system is a production-like engineering lab; production is not approved.

## One-minute introduction

“I built a Go/PostgreSQL distributed task execution system that focuses on correctness under retries, worker crashes, and concurrent ownership. The API persists typed tasks; workers claim bounded work using PostgreSQL row locking and leases. Each ownership generation has a fencing token so a stale worker cannot overwrite a newer result. Retry scheduling and task events commit transactionally. For external side effects, a durable effect ledger distinguishes intent, applied receipt, and unknown outcomes instead of assuming exactly-once delivery. The repository has real PostgreSQL integration and Race tests, Docker-based worker recovery, and evidence tied to commits. It is intentionally not production-approved: real provider credentials, cloud deployment, alert routing, and safe old-binary rollback still require work.”

## Three-minute version

“The core design is deliberately small: PostgreSQL is the durable scheduler and audit store; API instances are stateless; Workers scale independently. Claiming uses `FOR UPDATE SKIP LOCKED` under a transaction with queue-level governance, then advances a lease generation. A renewal loop keeps active work owned. If a Worker dies, expiry permits reclaim; the task can execute more than once, but the old generation cannot commit state because every terminal write is fenced by worker and lease version.

“Retries use database time for `next_run_at`, with capped exponential backoff and optional jitter. That distinction matters in tests: the pre-deadline assertion must prove the task is not claimable, while post-deadline polling is bounded because scheduler availability is eventual. Attempts that are exhausted or non-retryable go to DLQ.

“An external provider call cannot be atomic with our database transaction. We persist an effect intent and stable idempotency identity before the call; if the response is lost, the effect becomes unknown and requires provider reconciliation rather than a blind second charge/send. The HTTP provider is only a contract fixture, not a real payment vendor integration.

“The system has API contracts, OIDC/token boundaries, owner scoping, S3-compatible artifacts, tracing and metrics, but those do not automatically make it production-ready. We verified focused local failure paths and preserve manifests/logs outside the repo. We have not established production SLOs, multi-region durability, alert paging, signing, or migration rollback.”

## Five-minute version: useful design trade-offs

1. **Why PostgreSQL as queue?** It provides durable state, transactional events, row-level locking, fencing and governance in one system. The trade-off is a database-backed scheduling ceiling and pressure on indexes/connection pools. The 100K pagination benchmark is a local experiment, not a throughput promise.
2. **Why `SKIP LOCKED` plus a lease?** `SKIP LOCKED` prevents concurrent claimers from waiting on the same task row; a lease recovers work after process loss. A lease does not prevent duplicate execution when a handler exceeds its ownership or ignores cancellation, so downstream effects need idempotency and task writes need fencing.
3. **Why not promise exactly once?** The database cannot atomically commit with an external HTTP provider. The system records effect intent/receipt and surfaces uncertainty for reconciliation. A provider must support idempotency or lookup for safe automatic recovery.
4. **Why use database time?** Multiple API/Worker hosts may have clock skew. Retry due-time, lease expiration and shared-token refills should follow database authority rather than one process clock.
5. **What does MinIO prove?** It exercises the S3-compatible contract and crash-recovery path in isolated containers. It does not prove AWS IAM policy, KMS, region durability, bucket lifecycle, or production latency.
6. **What would you change for very large tenants?** Re-measure keyset query plans and owner-skew/write-cost under the target workload, then decide on indexes and partitioning. The current measured 100K local data set must not be extrapolated to production.
7. **How do you handle unknown external effects?** Keep the state explicit, expose it for an authorized reconciliation operator, preserve the audit trail, and never treat a timeout as proof the provider did nothing.
8. **How did you validate concurrency?** Use PostgreSQL integration, Go Race, independent API/Worker processes, and fault injection. Unit fakes alone cannot establish database lock or crash behavior.
9. **What is the rollback limitation?** We ran an isolated process matrix: Wave 3 API and Worker are ready on schema 11 but exit before serving on schema 12; Wave 4 API/Worker upgrade 11 to 12 and run on 12. Migration 012 is additive SQL, but the older binary's embedded-migration guard makes rollback incompatible. A pre-migration compatibility release or a rehearsed full restore/cutover is required; see the migration matrix report.
10. **What remains before production?** Choose and exercise the target platform, TLS and secret distribution, real providers, shared durable object store, alert ownership/paging, SLO/RPO/RTO, signing/provenance, retention approvals, backup restore, and compatible schema rollout.

## Evidence-backed measurements you may quote

- Package D local burst: 16 admitted / 48 rate-limited across two independently built API containers at configured burst 16. This is a bounded local E2E, not a rate-capacity or latency benchmark.
- Package E local pagination: 100,000 rows traversed in 201 keyset pages with zero duplicates, including a 50-row concurrent insert phase. The exact environment, query plans, percentiles and index write-cost comparison are in `WAVE4_PACKAGE_E_LIFECYCLE_AND_PAGINATION.md` and its E: evidence directory. Do not present those values as an SLO.
- Wave 4 F local alert E2E: one API target was stopped, its scrape-down rule fired, and it resolved after the API recovered. This is no paging/on-call proof.
- Wave 4 F local release smoke: an in-flight mock task completed at attempt 1 after Worker SIGTERM. This is no rolling-upgrade or rollback proof.

For deeper source paths see [`SYSTEM_OVERVIEW.md`](../architecture/SYSTEM_OVERVIEW.md), [`CRITICAL_CALL_CHAINS.md`](../call-chains/CRITICAL_CALL_CHAINS.md), the Package A–E reports, and the corresponding evidence manifests under `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave4-artifacts\`.
