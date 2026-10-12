# P0-A — Local runtime topology

Status: `IMPLEMENTED_STATIC / RUNTIME_NOT_VERIFIED / IMAGE_PIN_BLOCKED`.

## Implemented

- `deployments/local-first/compose.core.yml` defines isolated PostgreSQL 16,
  one-shot Migrator and post-migration role-grant job, MinIO plus a one-shot
  bucket/user initializer, Gateway Simulator, HTTPS JWKS fixture, API and
  Worker.
- Runtime service dependencies are explicit: PostgreSQL healthy → migrations
  completed → runtime grants completed → API/Worker. Gateway, identity, MinIO
  and MinIO initialization have separate readiness gates.
- PostgreSQL and MinIO have named project-scoped volumes. The private network
  is `internal`; only API and test identity are published and both bind to
  loopback. API and Worker use separate one-replica containers.
- Resource caps, restart policy, bounded health checks, stop grace periods and
  JSON log rotation are specified.
- `scripts/local-first/provision.ps1` creates random credentials and the test
  certificate hierarchy only under E:, refuses overwrite, sets user-only
  Windows ACLs, and does not print secret material.
- API TLS uses a distinct certificate with localhost SANs; API trusts the local
  test CA for JWKS. Identity and API get separate minimal certificate
  directories, and the CA private key is not mounted into either container.
- Gateway and MinIO credentials are mounted only to consumers that need them.

## Image provenance blocker

The Compose variable intentionally requires an explicit MinIO image reference.
The public MinIO GitHub repository reports itself as archived, and its project
is AGPL-3.0; Docker Hub search listings show historical `RELEASE.2025-09-07...`
tags but the Registry manifest request through the configured proxy returned
`UNAUTHORIZED`. The official Quay manifest request also returned `401`.
Consequently no registry manifest digest or anonymous fresh-runner pullability
was verified. Do not use the local image ID or an unreviewed mirror. Runtime
Compose remains blocked until an authorized, verifiable official source and
digest are provided. References: [MinIO repository](https://github.com/minio/minio),
[MinIO Docker Hub tags](https://hub.docker.com/r/minio/minio/tags).

PostgreSQL is pinned to the official multi-platform manifest
`postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`;
registry metadata resolved the amd64 child to PostgreSQL `16.15-alpine3.24`.
The MinIO root-user initialization contract, curl
healthcheck, and `mc`-based least-privilege setup still need runtime verification
against the final approved image.

## Validation

- `docker compose ... config --quiet`: PASS.
- 29 Compose topology checks: PASS.
- API/identity certificate CA and hostname/IP verification: 3/3 PASS.
- PowerShell AST, Bash syntax, MinIO policy JSON, targeted Go tests, and
  `git diff --check`: PASS.
- Sealed evidence: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\p0-runtime-static-20261011T135136+0800`
  (`f33c9c0b91c9c46f832adef6e1ab1a402859a6fc`, clean tree at run start).
- No Compose container was started: current available RAM is below the 6 GiB
  task gate (4.82 GiB at evidence capture), and no MinIO image is verified.
- Therefore service health, bind mounts, Linux container secret permissions,
  graceful shutdown and the end-to-end call chain remain `NOT VERIFIED`.

## Staged startup runner

`scripts/local-first/run-local-first.ps1` implements an ordered startup plan for
PostgreSQL readiness, migration, runtime grants, Identity/Gateway/MinIO
readiness, MinIO initialization, then API/Worker readiness. It requires a
pre-provisioned per-run `compose.env` and explicit run-scoped project ID;
`-Execute` is required before any Compose mutation. Without `-Execute`, it runs
the read-only preflight and records `PLAN_ONLY` or the blocking status.
Preflight evidence is E:-only, ACL-restricted, bound to a SHA-256 inventory of
changed source files, and sealed after files are closed. A blocked preflight
creates no network, container, volume, or service mutation. On an executed
startup failure, only the exact Compose project is stopped; named data volumes
are retained.

The runner currently verifies startup/readiness only. It does not invoke the
cross-process API → PostgreSQL → Worker → Gateway → MinIO → fenced-ACK
acceptance; see `P0_E_SYSTEM_E2E_RUNNER.md`. A separate
`cleanup-compose-run.ps1` now provides a dry-run-first, exact-Run-ID cleanup
path for stopping services, removing only that Compose project's containers
and network, and optionally deleting its generated ephemeral secret tree.
It rejects mismatched project/env/network/volume identities and unknown
resources, never passes `-v`, and preserves evidence. Its fake-executor tests
are offline control-flow evidence only; Docker cleanup has not been run.

Preflight smoke evidence:
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\compose-runs\valid-project-smoke-postredaction\run-20261011t140857p0800`.
Status is `RESOURCE_LIMITED`, not PASS: available RAM was 0.45 GiB, the MinIO
registry digest is unverified, and required images are absent locally. The
manifest records `executed=false`; no Docker Compose mutation occurred.
The recorded source inventory SHA-256 is
`0f62674d33ad112eb29a096c70fe87a73370413458a95d0f1177c688f850291b`; a
comparison against all generated secret files returned `SECRET_VALUE_LEAK=false`.

## Recovery boundary

`docker compose down --remove-orphans` for this explicit Compose project leaves
named volumes by default. The Cleanup runner verifies the exact known volume
names and reports zero volumes removed; database/object recovery is a separate
P0-F deliverable. Do not imply that volume persistence is backup or disaster
recovery.
