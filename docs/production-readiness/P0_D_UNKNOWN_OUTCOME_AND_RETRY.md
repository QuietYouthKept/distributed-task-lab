# P0-D — Gateway unknown outcomes and retry policy

The checked-in Gateway Simulator stores idempotency state in process memory.
The restart unit test intentionally proves that a new simulator has an empty
ledger and executes the same request again. This is not a commercial Provider
idempotency guarantee.

| Event | What this fixture proves | Safe action classification |
|---|---|---|
| Connection refused before request reaches Gateway | Not yet modeled by simulator; HTTP transport reports an error | Unknown unless transport proves no bytes/request were sent; reconcile or use a provider idempotency contract |
| Explicit HTTP 429 | `Retry-After: 1` mode returns before ledger begin | Retry only honoring bounded Retry-After and the task's retry budget |
| HTTP 500 | Simulator returns 500 before ledger begin | Provider-specific; do not infer non-execution from status alone |
| Client timeout after handler starts | In-memory entry is completed before the delayed response; execution count remains 1 | Unknown to caller; same request ID can replay within same process |
| Response lost after result creation | response-loss mode closes the socket after storing result in process memory | Same-ID retry is safe only while that in-memory ledger survives |
| Malformed response / invalid usage / cost limit | deterministic contract responses | Reject malformed/invalid/cost-over-limit result; don't silently ACK success |
| Gateway restart after execution | process memory ledger is lost; repeated request executes again | Unknown; external provider reconciliation or manual decision required if side effects may have occurred |
| Receipt stored in ObjectStore but ACK not committed | not established by simulator unit tests | Verify immutable artifact by key+hash, then fenced ACK; reclaim must not overwrite mismatched content |
| Worker cancellation | current tests do not prove the remote provider cancelled | Do not treat cancellation as proof of non-execution |

## Request identity invariant

An idempotency key is meaningful only if it is stable across retries and bound
to a request fingerprint. Same key/different payload must conflict. The
simulator has these in-process properties, including concurrent identical
requests (unit test), but it has neither durable state nor cross-instance
coordination. Production commercial-provider guarantees remain
`NOT VERIFIED`.

## Added test

`TestGatewaySimulatorConcurrentIdenticalRequestsExecuteOnce` starts eight
identical concurrent HTTP calls, bounds completion to two seconds, requires all
responses to succeed and asserts exactly one simulated execution. It is an
in-process fixture test, not a cross-process/durable-provider test.
