# Storage lifecycle

`tasks` is the live queue and `task_events` follows it through `ON DELETE
CASCADE`; neither is currently an immutable archive. Keep payloads/results
small and put large source documents, model outputs, and chunk manifests in
object storage, retaining their URI and content hash in task results.

Before applying retention, export terminal tasks and event history to a
versioned archive, verify counts and hashes, then delete in small indexed
batches by `finished_at`. For multi-million-row deployments, range-partition
terminal history by completion month and retain the active pending/running path
separately. API listing currently uses bounded offset pagination; add a stable
`(created_at,id)` cursor before exposing very large histories to users.

No retention period or purge policy is currently approved, and no delete job is
enabled. `cmd/retention-dry-run` is an inventory-only candidate report; it does
not establish object reachability or deletion safety. It excludes tasks with a
result, effect, or task security-audit reference, but operators must still
manually review task payloads and object-store references. Task deletion would
cascade `task_events`; those events are not an immutable archive.
