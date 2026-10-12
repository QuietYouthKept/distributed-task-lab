# P0-D — Gateway fault test matrix

| Fault | Fixture support | Test evidence level | Residual |
|---|---|---|---|
| Wrong API key / invalid contract | Yes | HTTP unit | None for fixture contract |
| HTTP 429 + Retry-After | Yes | HTTP unit | No production provider behavior |
| HTTP 500 | Yes | HTTP unit | Execution semantics must not be inferred from status |
| Client timeout | Yes | HTTP unit | Unknown outcome; same-process replay only |
| Response loss | Yes | HTTP unit | In-memory replay only |
| Malformed response | Yes | HTTP unit | Consumer validation is separate |
| Invalid usage / excessive cost | Yes | HTTP unit | Candidate validation is not real provider billing |
| Same ID, different payload | Yes | HTTP unit | Fingerprint uses request bytes; canonicalization is not promised |
| Concurrent same-ID calls | Added bounded HTTP unit | In-process only | No cross-process lock |
| Connection refused | Not simulated | NOT VERIFIED | Needs client transport fixture |
| Gateway restart | Yes | HTTP unit shows state loss | Restart loses dedupe state |
| Concurrent ID collision across processes | No | NOT VERIFIED | Requires durable/shared ledger or provider guarantee |
| Worker cancellation/crash and reclaim | Separate PostgreSQL/container E2E | NOT RUN here | Resource and MinIO gates apply |
| Artifact-before-ACK recovery | Existing E2E contract, not run here | NOT RUN | Real S3 required |

No Gateway fault was injected into an active container during this checkpoint.
