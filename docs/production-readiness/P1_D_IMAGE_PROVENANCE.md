# P1-D — Image provenance inventory

| Image | Source/reference | Pin type | Verified here | Limitation |
|---|---|---|---|---|
| PostgreSQL | `postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` | OCI multi-platform index digest | Prior checkpoint registry inspection | No current pull/build in this checkpoint |
| MinIO | required environment value; no fallback | none accepted | NO | Official Docker Hub manifest request returned 401 `insufficient_scope`; no digest/platform metadata |
| API/Worker/Migrator/Gateway/Identity | local Dockerfile builds | local image identity only after build | NO current build | Build, SBOM, scan, signature, provenance pending |

An Image ID is not a Registry Manifest/Index Digest. Local cache availability
cannot establish registry ownership, platform manifest, or provenance.
