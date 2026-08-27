# Interview Q&A

**Why is a worker ID not the correctness boundary?** It aids observability; the lease version is the fencing credential. Two processes configured with the same ID still cannot mutate a newer generation using an older version.

**Why lock a queue row as well as task rows?** Task locks stop duplicate ownership of one task. They cannot serialize the aggregate `running(queue) <= max_concurrency` decision.

**Why `SKIP LOCKED`?** It avoids waiting on a contended queue authority and lets independent queues make progress. The trade-off is temporary starvation of a continuously contended queue.

**Why not call this exactly once?** A handler can perform an external effect and die before it records success. Lease replay can then re-execute it; a stable business key or outbox is needed at the effect boundary.

**Where is time authoritative?** PostgreSQL `clock_timestamp()` decides lease expiry, scheduling, retry eligibility, and token refill. Go time governs local polling and deadlines only.

**What does `go test -race` prove?** It detects Go memory races. PostgreSQL/MVCC logical races are covered separately by real integration, Asset Lab, and fault scenarios.
