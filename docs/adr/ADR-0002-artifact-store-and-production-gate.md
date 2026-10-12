# ADR-0002: Shared artifact store and production worker gate

- Status: Accepted for Phase 1 implementation; adapter contract still requires MinIO verification
- Date: 2026-10-09
- Scope: TASK-012, TASK-013, TASK-014

## Decision

Worker file access is represented by a context-aware `artifacts.Store`. Local
filesystem access is rooted, object keys reject traversal/URL forms, writes
are bounded, and content is checked by SHA-256. The S3-compatible adapter uses
a single conditional PutObject (`If-None-Match: *`) and explicit SHA-256 object
metadata; ETag is not treated as a checksum. The configured bucket is not
automatically created unless explicitly allowed for local development.

Production worker startup remains fail-closed until all artifact-producing
handlers use the shared adapter and the selected MinIO/S3 service contract,
cross-process visibility, cancellation, and ambiguous-write recovery have
passed integration tests. MinIO is not interchangeable with arbitrary S3
vendors; each deployment target requires its own verification.

## Consequences

- Real RAG ingestion now accepts injected source and destination stores.
- Eval, replay, and effect receipt paths still use filesystem operations and
  therefore prevent removing the production gate.
- No claim of MinIO contract success is made until its integration suite runs.
