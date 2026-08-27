# Final Review Guide

Review these items before release:

1. `ClaimTasks`: queue lock before task lock; fresh capacity count after lock.
2. Queue `SKIP LOCKED`: no cross-queue head-of-line blocking; accepted starvation trade-off.
3. Reclaim path: expired running work replaces, not consumes, capacity.
4. Rate path: transaction contains only decision/reservation, never a sleep.
5. `RenewLease`: full owner credential and expiry predicate.
6. `MarkSuccess` and `MarkFailure`: zero rows become `ErrTaskNotClaimed`.
7. Pool renewal cancellation: local completion is not distributed ownership loss.
8. Pool shutdown: claims stop before drain; renewals persist through cooperative drain.
9. Migration lock, checksum, gap, and transaction behavior.
10. Migration 007 constraints: no historical-row deletion or silent repair.
11. API public health endpoints and authenticated task/admin boundary.
12. No authorization/payload secrets in logs, metrics, or trace attributes.
13. Metrics labels contain no task IDs, worker UUIDs, or raw error strings.
14. README guarantee wording matches `docs/guarantees.md` and the code.
15. `TL-CLAIM-002`, `TL-QUEUE-001`, `TL-FENCE-002`, `TL-RATE-003`, and `TL-CRASH-002` raw evidence.
16. Provenance matrix distinguishes reference concepts from NATS/Redpanda protocol claims.
17. Evidence checker v2 reports `independent_oracle=true` for its five recomputed experiments.
18. Dockerfiles use runtime-safe non-root execution and do not bake tokens.
19. `.gitignore` excludes local Round snapshots and extracted source material.
20. Release notes retain the at-least-once and single-PostgreSQL boundaries.
