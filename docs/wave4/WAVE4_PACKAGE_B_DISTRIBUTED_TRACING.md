# Wave 4 Package B — Distributed Tracing

Status: `PASS — LOCAL PACKAGE ACCEPTANCE`

Implementation commit: `8099c68fd1448999cf3a262672519967d0a1d867` (tree `9eb8bde1820462f7fb3353810bc2bac20baa18df`), based on Package A commit `2069ce1406b8626f003e72266f558600926db2a0`. All results below are bound to the clean Package B source commit; hosted CI remains unverified.

Production remains `PRODUCTION_NOT_APPROVED`; this package adds diagnostic instrumentation and does not enable production task execution.

## What changed

- API-supplied trace context is stored only after the API removes client-provided `_traceparent` and `_tracestate` fields and injects its own active server span. Strict task-type validation now recognizes those two reserved fields, validates the traceparent, temporarily removes the fields from business-schema validation, and restores them to the canonical payload. Other unknown business fields remain rejected.
- Each Worker execution is an independent consumer trace with a link to the producer context persisted in the task. It is not represented as a synchronous child across the queue wait. The final ACK/failure write detaches task cancellation while retaining the active worker span and uses a bounded timeout. Error spans retain a safe error type/status but omit raw error text, which can contain object keys, request data, or credentials.
- PostgreSQL enqueue/claim/completion/failure/renewal spans record safe task, lease, retry, fencing, and commit metadata, but not raw failure text. S3 `Head`/`PutIfAbsent` spans record operation and byte count without object keys or payloads.
- The optional cross-container tracing harness starts a digest-pinned OpenTelemetry Collector only for `-Tracing`, polls for the expected exported spans for a bounded 20 seconds, redacts service credentials, verifies a synthetic sensitive-payload marker is absent from Collector output, and writes run-specific evidence under E:.

## Root cause found in real E2E

The first new isolated run exposed a production validation defect: the API correctly replaced client trace fields with its trusted server span, but `taskpayload.ValidateData` then rejected `_traceparent` as an unknown RAG field and returned HTTP 422. The fix is confined to reserved propagation metadata; arbitrary unknown task fields still fail validation. A regression test covers valid metadata preservation, malformed metadata rejection, and continued unknown-field rejection.

The first post-fix run reached a separate harness assertion failure: the test expected a hard-coded HTTP span name that `otelhttp` does not emit, and collected spans before the exporter's batch delay elapsed. The harness now checks actual emitted HTTP method/path attributes and uses a bounded collector-output poll. This was a test-harness correction, not a relaxation of the span contract.

## Evidence

The first successful run established API → PostgreSQL → two Workers → MinIO → Collector export and trace-link correlation. The next run also executed the real Worker-A crash-after-PUT/before-ACK recovery test with tracing enabled: the owner was killed, Worker B reclaimed attempt 2 / lease version 2, verified the same durable artifact, and completed the task. Collector output contains the task ID, `expired_lease` reason, worker execution span, and link to the API producer span. Both runs used fresh Compose project names, ephemeral credentials, and separate evidence directories.

Final clean-source evidence directories (outside the D: repository):

- `E:\distributed-task-lab-temp\wave4-artifacts\package-b-final-8099c68-20261010` — full unit, full Race, vet, required PostgreSQL integration logs, manifest, and SHA-256 list.
- `E:\distributed-task-lab-temp\wave4-artifacts\tracing-e2e-runs\evidence\tasklab-e2e-4df8581dda83` — final clean-source Collector + crash/reclaim E2E, two passing checks, per-file SHA-256 values, and redacted service logs.

Earlier successful dirty-source runs (useful diagnostic evidence, not the clean-source acceptance basis):

- `E:\distributed-task-lab-temp\wave4-artifacts\tracing-e2e-runs\evidence\tasklab-e2e-7b22580a0bfc` — tracing happy path; initial post-fix run; source was dirty and its manifest records that fact.
- `E:\distributed-task-lab-temp\wave4-artifacts\tracing-e2e-runs\evidence\tasklab-e2e-491d9ebd5983` — tracing plus two-worker crash recovery; source was dirty and its manifest records that fact.
- Failed attempts are preserved under `E:\distributed-task-lab-temp\wave4-artifacts\tracing-e2e-runs\attempts`; they are not rewritten as passing results.

The Collector is `otel/opentelemetry-collector-contrib@sha256:89107a3a8f4636a396927edf7025bb9614b8da2d92f4cc3f43109e8d115736e2` (release 0.135.0; upstream source `open-telemetry/opentelemetry-collector-releases`; Apache-2.0 metadata). The manifest records the immutable digest. This local E2E proves receipt by the Collector debug exporter, not retention, alerting, or a production trace backend.

## Limits and follow-up

- Full unit, full Race, `go vet ./...`, required PostgreSQL integration, and the clean-source traced crash/reclaim E2E all passed. PostgreSQL integration included `TestRetryBackoff` and `TestRetryTaskReclaimable`; the opt-in backup/restore drill remained skipped because `RUN_BACKUP_RESTORE_DRILL=1` was not enabled.
- The scoped E2E proves delivery to the Collector debug exporter and redaction for that run. It does not prove durable retention, alerting, cross-region delivery, or production trace-backend operations.
- Hosted CI has not been run for the Wave 4 branch. No production readiness or availability claim is made.
