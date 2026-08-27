# Interview Demo Runbook (8 minutes)

1. Set distinct `API_TOKEN` and `ADMIN_TOKEN`, then run `make up`.
2. Show `GET /live` and `GET /ready`; explain liveness versus schema/DB readiness.
3. Submit one RAG task with the API token and show pending → running → success.
4. Configure a queue with the admin token: cap 1 and a small rate; show ordinary token cannot perform this mutation.
5. Run `make demo-rate-limit`; explain queue-row authority and the DB-backed token bucket.
6. Open `docs/review/INVARIANT-MAP.md`; explain task row ownership versus queue aggregate admission.
7. Run Failure Lab or show its JSON timeline: hard crash recovers by lease generation, not by worker identity.
8. Show `TL-FENCE-002` and the checker v2 output; close with the at-least-once boundary.

Do not claim exactly-once effects, replicated durability, or starvation freedom. The strongest concise statement is: PostgreSQL serializes task ownership and queue admission, while handlers remain responsible for idempotent external effects.
