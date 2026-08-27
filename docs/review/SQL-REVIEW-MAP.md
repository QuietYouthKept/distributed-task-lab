# SQL Review Map

| Operation | Transaction and locks | Fencing / expected outcome |
|---|---|---|
| Enqueue | transaction; queue upsert then task insert | idempotency conflict returns canonical task |
| Claim | transaction; lock queue authorities with `SKIP LOCKED`, then candidate task rows | lease version increments; stale candidates are skipped |
| Capacity/rate | same claim transaction; fresh `READ COMMITTED` count after authority lock | cap decision and token reservation are serialized; no sleep while locked |
| Renew | single fenced `UPDATE` | zero rows is `ErrTaskNotClaimed` |
| Success | transaction; fenced state update then event | zero rows is `ErrTaskNotClaimed` |
| Failure | transaction; fenced `SELECT FOR UPDATE`, retry or DLQ update and event | zero rows is `ErrTaskNotClaimed` |
| Pause/config | queue row update | serialized against claim by the same queue authority row |
| Migrations | advisory lock and one transaction per migration | checksum/gap/drift failure aborts without partial migration |

The intentional liveness trade-off is queue-authority `SKIP LOCKED`: a queue whose authority is continuously contended can be skipped temporarily so it does not block unrelated queues. Priority is strict and has no aging mechanism.
