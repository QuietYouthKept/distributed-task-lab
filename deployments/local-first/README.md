# Local-first production-like runtime

This is an isolated `staging-sim` environment, not production. It exercises a
local PostgreSQL database, test JWKS issuer, Gateway Simulator, MinIO and
separate API/Worker processes. It does not enable a production execution
allowlist, payments, notifications, hosted identity, or production credentials.

## Provisioning and static validation

Run in PowerShell from the repository root:

```powershell
.\scripts\local-first\provision.ps1
```

The script creates a unique E: directory with random database/service
credentials, a short-lived test CA and distinct identity/API certificates. It
prints only the resulting directory and Compose env-file paths, never secret
values. Keep that directory outside source control. Windows ACLs restrict it
to the current user; this does not establish that Docker Desktop enforces
equivalent in-container Linux file permissions, which remains a runtime check.

Before using the Compose file, replace the mandatory `MINIO_IMAGE` value in the
generated `compose.env` with a verified official immutable reference
(`repository:release-tag@sha256:<registry-manifest-digest>`). Do not substitute
a local Docker image ID. Then run:

```powershell
docker compose --env-file <compose.env path> -f deployments/local-first/compose.core.yml config --quiet
```

The MinIO upstream repository is archived and the registry endpoints tested
from this host did not provide an anonymously pullable manifest. No third-party
mirror or local cache is silently substituted. Until an official image and
digest can be verified and fetched, starting the runtime is blocked.

## Runtime boundary

Only `api` and `identity` publish ports, bound to `127.0.0.1`. PostgreSQL,
MinIO and the Gateway have no host-published ports. The core network is
internal-only. Compose uses explicit resource caps and bounded health checks.
The API and Worker receive separate PostgreSQL login roles and secret-file
DSNs; the one-shot migrator uses its own role. Runtime grants are applied only
after migration and grant no DDL, DELETE, role, or database-creation privilege.

The Worker is only configured for the checked-in `staging-sim` execution
policy; this does not change production fail-closed behavior. The local JWT
issuer is a protocol fixture, not a complete OIDC browser-login service. The
Gateway is simulated, and all object storage is local MinIO.

Do not start Compose E2E unless `scripts/local-first/check-resources.ps1
-Workload container -ReportOnly` reports the container gate allowed (at least
6 GiB available RAM, TIME_WAIT at most 5,000, at least 20 GiB free on E:, CPU at
most 90%, Docker available). Never clean other projects' Docker resources.

## Scoped teardown

After this profile has been run, inspect `docker compose ... ps -a` first. Use
`docker compose ... down` only for this exact project/file. Do not use
`down -v`, global prune, or remove resources by broad name; preserving the two
named volumes is the default. Any disposable test data reset needs a separately
recorded, explicit target check.
