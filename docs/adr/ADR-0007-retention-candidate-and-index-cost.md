# ADR-0007: Dry-run retention boundary and pagination index cost

- Status: Proposed; no purge policy or deletion path is approved.
- Date: 2026-10-10.

## Context

`task_events.task_id` uses `ON DELETE CASCADE`, so deleting a task also removes transition history. That history is not an immutable compliance archive. `effects.task_id` uses `ON DELETE RESTRICT`; external-effect evidence must not disappear with task cleanup. `security_audit_events` is append-only but references task IDs by `resource_type/resource_id` without a foreign key. Artifact references are embedded in task payload/results rather than a normalized reference table, and stored objects may outlive the task row.

There is no approved retention duration, archive format, audit retention rule, artifact object ownership protocol, or restore contract. Consequently a purge could orphan or erase evidence.

## Decision proposal

Add only a read-only candidate inventory. It selects terminal tasks (`success`, `failed`, `dlq`) finished before an explicit cutoff, with no result, no effect row, and no security audit reference; it does not mutate rows or objects. It uses a read-only transaction, PostgreSQL clock validation, five-second statement timeout, and a maximum 1,000-row page. Tasks with results are conservatively omitted because they may carry artifact references. Every report warns that payloads and object-store references still require manual verification and that a future task delete cascades event history.

The operator command requires `RETENTION_DRY_RUN=true`, an explicit cutoff, and a database URL. It has no delete mode. Retention duration, archive-before-delete, audit policy, artifact reachability, resumable purge, and concurrent-worker policy must be approved and designed separately before any deletion feature exists.

For PostgreSQL performance analysis, keep the 100K keyset walk and add a skewed owner distribution, full task-row `EXPLAIN ANALYZE BUFFERS`, index sizes, and same-run temporary-table write-cost comparison. Results are local experiments, not SLOs. No production index is changed by this ADR.

## Consequences

- Dry-run evidence identifies a conservative review set but cannot prove deletion safety or object reachability.
- No task/event/effect/audit/artifact data is deleted by the command.
- The benchmark exposes read plans and write/index-size tradeoffs without claiming an index should be removed.
- Any future purge requires separate approval and a tested archive/restore, audit, object cleanup, failure resume, and rollback plan.

Implementation and evidence: `docs/wave4/WAVE4_PACKAGE_E_LIFECYCLE_AND_PAGINATION.md`; local benchmark outputs live under `E:\\distributed-task-lab-temp\\wave4-artifacts`.
