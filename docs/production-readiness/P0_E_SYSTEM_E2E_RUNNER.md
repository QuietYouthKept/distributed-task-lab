# P0-E — bounded local system E2E runner

Status: `STARTUP_RUNNER_IMPLEMENTED; BOUNDED_BUSINESS_E2E_RUNNER_IMPLEMENTED_NOT_VERIFIED`.
Cross-process acceptance is `NOT VERIFIED`; local execution is also
`RESOURCE_LIMITED + MINIO_IMAGE_SOURCE_BLOCKED`.
`PRODUCTION_NOT_APPROVED` remains unchanged.

## Existing execution boundary

`scripts/local-first/run-local-first.ps1` is a phased startup/readiness runner,
not a task acceptance runner. It has an explicit `-Execute` gate and now
captures staged, unstaged, deleted, and untracked source changes in its source
inventory. It must not be described as proving
OIDC → API → PostgreSQL → Worker → Gateway → MinIO → fenced ACK.

`scripts/local-first/cleanup-compose-run.ps1` is a separate cleanup operation.
It defaults to dry-run, requires a Compose Run ID that exactly matches the
environment file, verifies Docker ownership labels before Apply, retains the
two data volumes, and does not remove the original evidence. See
`test-compose-cleanup.ps1`; that Fake Executor suite does not contact Docker.

`scripts/local-first/run-system-e2e.ps1` is the bounded business acceptance
entry point. It requires explicit `-Execute`, passes the image/resource
preflight before requesting a token, validates the local test CA in-process
without global certificate-store changes, submits exactly one idempotent
`eval/gateway` task, polls with capped exponential backoff and a hard deadline,
requires successful status plus claim/success events and a nonzero lease
version, then reads the exact manifest and case-results bytes through an
ephemeral run-scoped MinIO client container and hashes the original bytes.
Tokens and secret values are not persisted. `-StopServicesAfter` is opt-in and
stops only the exact Compose project; it never removes volumes.

## Required bounded acceptance sequence

The following are required assertions for a future E2E execution. No task is
submitted by this document or by the startup runner:

1. Re-run `preflight.ps1` against the exact Compose project and fail before
   mutation unless resource, immutable image, local image, certificate, secret,
   and project identity checks all pass.
2. Verify `GET /ready` over HTTPS with the provisioned test CA and API SAN.
3. Obtain a short-lived `operator` fixture token over the local identity
   service's HTTPS endpoint. The token issuer is a test fixture, not production
   OIDC login.
4. Submit one schema-valid `eval/gateway` task over HTTPS with a unique
   idempotency key. Record the request fingerprint without logging the token or
   payload secrets.
5. Poll `GET /v1/tasks/{id}` with bounded exponential backoff and a hard
   deadline. Success means a terminal success state, not HTTP 200.
6. Fetch task events and require a coherent claim/attempt/lease transition and
   fenced completion. Reject stale lease generations.
7. Verify the result artifact by fetching the exact bucket/key referenced by
   the task, streaming bytes to SHA-256, and matching the manifest run ID and
   fingerprint to the task result. Hashes are recorded in the sealed E: run
   bundle; the task API does not currently persist an independent SHA field.
8. Record API, Worker, Gateway, PostgreSQL, and object-store evidence with
   secrets/payload redaction. Close all writers before hashing the run bundle.
9. Stop or remove only the exact test Compose project according to an explicit
   operator choice; never remove named data volumes as part of E2E cleanup.

## Not yet verified

- No trusted immutable MinIO registry digest is currently available; the
  compose image remains a placeholder and runtime preflight must fail closed.
- Container resource gate and startup readiness have not passed at current
  resource conditions; no services were started in this increment.
- No operator token, API request, task execution, event read, artifact fetch,
  fenced ACK, crash recovery, or cross-process trace has been performed.
- The Fake Executor cleanup suite proves only that control flow is scoped and
  volume-preserving; it is not Docker E2E evidence.

## Execution command after gates recover

First use the explicit startup runner with a provisioned run-scoped env file:

```powershell
pwsh -NoProfile -File scripts/local-first/run-local-first.ps1 `
  -ComposeEnvFile <REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\secrets\<run>\compose.env `
  -ComposeProject tasklab-local-first-<run> -Execute -KeepRunning
```

Then run the bounded acceptance explicitly. It stops before token issuance while
any preflight blocker remains:

```powershell
pwsh -NoProfile -File scripts/local-first/run-system-e2e.ps1 `
  -ComposeEnvFile <REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\secrets\<run>\compose.env `
  -ComposeProject tasklab-local-first-<run> -Execute -StopServicesAfter
```

No E2E request has been issued in this checkpoint, so system E2E remains
`NOT VERIFIED`. When the run is complete, explicitly choose service stop and/or
project container/network removal. Example dry-run first:

```powershell
pwsh -NoProfile -File scripts/local-first/cleanup-compose-run.ps1 `
  -ComposeEnvFile <REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\secrets\<run>\compose.env `
  -ComposeProject tasklab-local-first-<run> -StopServices -RemoveContainersAndNetwork
```

Only after inspecting the printed plan should an operator pass `-Apply
-ConfirmRunId <run>`; deleting the run's generated secrets additionally
requires `-DeleteEphemeralSecrets`. Data volumes remain retained.
