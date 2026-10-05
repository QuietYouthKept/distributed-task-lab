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
