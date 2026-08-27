# Invariant Map

| Invariant | Authority | Code / SQL | Tests and experiment | Known limitation |
|---|---|---|---|---|
| No concurrent same-generation claim | PostgreSQL task row lock | `ClaimTasks`; `FOR UPDATE SKIP LOCKED` | concurrent claim; `TL-CLAIM-002` | expiry can cause later execution |
| Queue cap | queue row lock | fresh running count under queue authority | capacity integration; `TL-CAP-003` | `0` is unlimited |
| Independent queues | queue-row `SKIP LOCKED` | authority selection before task locking | contention regression; `TL-QUEUE-001` | contended queue can starve |
| Stale owner cannot mutate | fencing token | success/failure/renew predicates | fencing tests; `TL-FENCE-002` | external effect cannot be undone |
| Shared rate admission | queue row lock | atomic token refill/reservation, no DB sleep | rate integration; `TL-RATE-003` | applies to admission, not completion |
| Crash recovery | DB lease expiry | reclaim increments generation | lease/fault tests; `TL-CRASH-002` | at-least-once work |
| Graceful drain | pool lifecycle | stop claims, retain renewal, drain | pool tests; `TL-SHUT-002` | uncooperative handler may be abandoned |
