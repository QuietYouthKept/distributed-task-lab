# P0-G — Observability acceptance boundary

Existing source includes structured logging/redaction, application metrics,
OpenTelemetry tracing, an OTLP collector profile, Prometheus rules and alert
fixtures. Existing unit tests cover selected logger/metric/trace behavior.
This checkpoint did not launch Collector, Prometheus, Alertmanager, or a test
receiver; it does not establish cross-process export or alert delivery.

## Cardinality and sensitive-data review

Prometheus labels must remain finite (status class, queue class, operation,
bounded error category). Do not label metrics with task ID, principal/user ID,
object key, request body, or raw URL. Traces/logs must not include bearer
tokens, DSNs/passwords, object-store credentials, payload bodies, or JWTs.
Source review is not a substitute for inspecting emitted live telemetry.

## Acceptance sequence

1. Unit tests for metric registration and redaction.
2. In-process OTLP receiver: producer context and safe attributes.
3. Separate API/Worker/Collector containers: trace continuity and span links.
4. Prometheus rule unit test for Normal -> Firing -> Resolved.
5. Local test receiver confirms delivery; capture receiver and alert logs.

Current statuses: selected unit/source review PARTIAL; cross-process OTLP and
alert delivery NOT VERIFIED; production on-call/approval NOT VERIFIED.
