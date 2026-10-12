# ADR — Object-store backend for local acceptance

Status: **Proposed / decision required**  
Date: 2026-10-11

## Context

The local-first Core profile uses the MinIO S3 API. An authenticated Docker Hub
Registry v2 challenge flow for the official `minio/minio` repository returned
`401 insufficient_scope`; no manifest digest or platform index could be
verified. The official upstream source is archived and AGPL-3.0. The Compose
file therefore keeps the image mandatory and does not fall back to local cache.

## Options

1. **Official MinIO image, authenticated pull.** Preserve current semantics;
   user performs registry authorization privately, then the registry manifest
   and digest can be verified. License and maintenance status still need
   acceptance.
2. **Reproducible local build from an official source release.** Avoids an
   unreviewed registry mirror but requires pinned source, build provenance,
   vulnerability review, and AGPL decision. Not implemented.
3. **Another S3-compatible server.** Could unblock a protocol contract suite,
   but it is not MinIO-equivalent and requires license, feature, consistency,
   and SDK compatibility review. Not selected.
4. **ArtifactStore fake/contract-only tests.** Useful for isolated application
   logic; cannot establish S3 or multi-process object persistence. Not a
   replacement for the required real S3 acceptance.

## Decision

No substitute is selected in this checkpoint. Keep `MINIO_IMAGE` required and
mark all real MinIO acceptance `DEPENDENCY_BLOCKED`; preserve S3 contract tests
for later execution. A user decision is required only if choosing a different
image source, license posture, or object-store implementation. An authenticated
pull of the existing official source is sufficient to retry source verification.

## Consequences

No MinIO-backed E2E PASS may be reported. PostgreSQL-only acceptance, scripts,
security tests, and offline tooling remain independently useful. Local
production-like evidence must remain distinct from cloud or production
acceptance.
