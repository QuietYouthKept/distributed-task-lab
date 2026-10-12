# P0-A — MinIO image source and registry evidence

Snapshot: 2026-10-11 (Asia/Shanghai). Source commit before this report:
`8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a`.

## Decision

`MINIO_IMAGE_SOURCE_BLOCKED`. `deployments/local-first/compose.core.yml`
continues to require `MINIO_IMAGE` and rejects an unset value. No image was
pulled, built, or substituted. The Docker Image ID previously seen locally is
not accepted as an OCI registry manifest digest.

## Bounded OCI authentication diagnostic

Proxy: `http://127.0.0.1:7890`. The request used Docker Registry v2's Bearer
challenge flow against the official Docker Hub repository:

| Step | Result |
|---|---|
| Manifest URL | `https://registry-1.docker.io/v2/minio/minio/manifests/RELEASE.2025-02-07T23-21-09Z` |
| Initial manifest | HTTP 401; `WWW-Authenticate: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:minio/minio:pull"` |
| Token endpoint | `https://auth.docker.io/token?service=registry.docker.io&scope=repository%3Aminio%2Fminio%3Apull`; HTTP 200; token value intentionally not recorded |
| Authenticated manifest | HTTP 401; `error="insufficient_scope"`; body says `UNAUTHORIZED`, authentication required, requested repository action `pull` |
| Manifest media type/digest/platform | Not available because the authenticated request was rejected |

This is an authorization/scope failure, not evidence that the tag is absent.
No OCI index or platform digest was obtained; no AMD64/ARM64 availability is
claimed. The two bounded network attempts are exhausted for this work package.

## Provenance and license boundary

- Investigated: official Docker Hub repository `minio/minio`; this host did
  not obtain a pull-authorized manifest.
- Previously investigated in the preceding checkpoint: official Quay endpoint
  also did not yield an anonymously retrievable manifest. This is historical
  diagnostic context, not a current digest verification.
- Not verified: release tag existence, registry digest, index digest, child
  platform manifests, release date from registry metadata, or image provenance.
- The upstream source repository is archived and carries AGPL-3.0. Local use
  may be possible, but redistribution/service obligations and maintenance risk
  require a deliberate license review; an archived project is not equivalent
  to an actively maintained dependency.
- Not accepted: local cache/Image ID, `latest`, unknown mirrors, unreviewed
  AIStor images, or another S3 server silently substituted as equivalent.

## Next decision

No cloud cost is required to resume. A user-approved, authenticated official
registry pull (without sharing credentials with this tool), or a reviewed
reproducible source-build and license plan, would permit renewed verification.
Changing the object-store implementation or accepting an alternate image
requires an explicit decision. Until then, MinIO-dependent Compose E2E remains
blocked; PostgreSQL-only and offline work may proceed independently.
