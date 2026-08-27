# Ownership Mutation Audit

Owner-only transitions use the `(task_id, worker_id, lease_version)` fencing credential. A zero-row update is returned as `domain.ErrTaskNotClaimed`; the worker must discard its result rather than retrying that ownership write.

| Operation | Location | Owner credential | Zero-row behavior | Coverage |
|---|---|---|---|---|
| Claim / reclaim | `task_repository.go:ClaimTasks` | task row locked; increments `lease_version` | candidate skipped or transaction rolls back | concurrent claim, capacity, queue tests |
| Renew lease | `RenewLease` | ID, worker, version, unexpired lease | `ErrTaskNotClaimed` | stale renewal test, `TL-FENCE-002` |
| Mark success | `MarkSuccess` | ID, worker, version, `status='running'` | `ErrTaskNotClaimed` | stale success test, `TL-FENCE-002` |
| Mark failure / retry / DLQ | `MarkFailure` | initial `SELECT ... FOR UPDATE` uses ID, worker, version, running state | `ErrTaskNotClaimed` before mutation | stale failure test, `TL-FENCE-002` |
| Expired final-attempt DLQ | `ClaimTasks` | no live owner exists: predicate requires expired lease | not applicable | lease/retry integration tests |
| Pause/resume/config | queue methods | queue authority, not task ownership | not found is an error | pause linearization tests |

`Requeue` and `Cancel` are administrative transitions, not worker ownership transitions; they are protected by the admin bearer boundary.
