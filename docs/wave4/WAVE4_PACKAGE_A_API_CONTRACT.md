# Wave 4 Package A — API Contract Engineering

Status: `IMPLEMENTED; PACKAGE ACCEPTANCE PENDING`

Source baseline: `a8656a7b72d792986775d94fe007acc7771c072b` (tree `14675a4cf091e87d6eb629b22d151eb35816fd92`). This report describes Wave 4 changes until an implementation commit is recorded. It does not re-label Wave 3 evidence as validation of these changes.

## Scope and coverage

The stable v1 surface contains 15 operations. The OpenAPI document now declares a success response schema, error responses, role metadata, parameters, and request-body semantics for each:

| Operation | Success shape | Required role |
|---|---|---|
| `GET /v1/tasks` | `TaskListResponse` | reader |
| `POST /v1/tasks` | `CreateTaskResponse` | operator |
| `GET /v1/tasks/{id}` | `Task` | reader |
| `GET /v1/tasks/{id}/events` | `TaskEvent[]` | reader |
| `POST /v1/tasks/{id}/cancel` | `TaskActionResponse` | operator |
| `POST /v1/tasks/{id}/requeue` | `TaskActionResponse` | operator |
| `GET /v1/dlq` | `Task[]` | operator |
| `GET /v1/stats` | `TaskStatsResponse` | operator |
| `GET /v1/metrics` | Prometheus `text/plain` | operator |
| `GET /v1/queues` | `QueueListResponse` | operator |
| `PUT /v1/queues/{name}` | `QueueActionResponse` | admin |
| `POST /v1/queues/{name}/pause` | `QueueActionResponse` | admin |
| `POST /v1/queues/{name}/resume` | `QueueActionResponse` | admin |
| `GET /v1/effects` | `EffectsListResponse` | operator |
| `POST /v1/effects/{id}/resolve` | `EffectResolutionResponse` | admin |

The JSON DTOs used by API handlers are named in `internal/api/contract/v1.go`. Existing v1 JSON keys have been preserved. In particular, `effects.Record` has no JSON tags and its existing capitalized field names remain documented rather than silently changing the wire format. Task IDs remain strings (storage allows non-UUID legacy identifiers); effect IDs remain UUIDs.

## Drift checks

`internal/api/openapi_v1_drift_test.go` checks the stable route/method set against `router.go`, exact parameter and response-status catalogs for all 15 operations, operation role metadata, response references, DTO JSON property names/types/required fields, pointer nullability, enums, and the shared error envelope. Existing authentication and owner-isolation tests remain in place; PostgreSQL integration is still required for package acceptance.

`internal/api/handlers/effect_handler_test.go` verifies the nil-ledger path returns the common JSON error envelope, request ID, and the existing 503 status. This changes only the unavailable/misconfigured handler body from plain text to the documented error representation; no success response or status is changed.

## Compatibility details and limits

- Create-task `type` is optional and accepts null/empty as the handler's existing default. Payload remains required.
- Optional cancel/requeue reason and queue-config numeric pointers are documented as nullable in line with Go JSON decoding and handler defaults.
- Metrics success is Prometheus text, not JSON. Its provider failure is text/plain 500 and is documented separately from the common JSON error envelope.
- Queue configuration is an upsert, so the contract does not claim a 404 response for that operation.
- The API ingress limiter remains process-local; the OpenAPI description does not imply a cross-replica quota.
- DTO reflection checks cover Go-visible JSON shape, not every semantic validation rule or middleware ordering. Request/response integration tests and owner-isolation tests remain necessary.
- OpenAPI `maxLength` counts Unicode characters while Go handlers may enforce byte limits. This contract does not assert those are equivalent; byte-specific constraints need explicit vendor extensions or a future API policy decision.

## Evidence and gate status

Resume-run evidence is stored under `E:\distributed-task-lab-temp\wave4-artifacts\package-a-resume-20261010`:

- `go test ./internal/api/... -count=1`: PASS.
- `go test -race ./internal/api/... -count=1`: PASS.
- `go vet ./internal/api/...`: PASS.
- `go version`: Go 1.26.9 on Windows/amd64.

These are package-targeted checks, not the required full unit, full race, or PostgreSQL integration gates. Package A remains pending those gates and a clean-source final evidence run. Raw logs and their SHA-256 manifest are on E: and are not committed as if they validated a later commit.
