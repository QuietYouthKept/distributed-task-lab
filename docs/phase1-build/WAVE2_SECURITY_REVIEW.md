# Wave 2 security review

## Reviewed boundaries

- **JWT/JWKS, RBAC, IDOR:** code uses verifier-created principals and explicit claim mappings; resource ownership, static-token restrictions in production config and audit transaction tests are covered locally. External IdP/JWKS rotation was not exercised.
- **Artifact references:** object keys are normalized relative keys; S3 endpoint/bucket/prefix/credentials come from server config, not task payload. Logical keys no longer expose configured prefix. S3 requires TLS except explicit development HTTP.
- **Artifact access:** the pinned MinIO contract now checks bad credentials, missing objects, concurrent conflicting writes, and cancellation. It passed against a temporary local MinIO instance using per-run random credentials; the test does not prove production IAM/policy isolation or AWS S3 behavior.
- **Secret leakage:** CI generates random MinIO credentials per job, stores them only in the ephemeral runner/container environment, and removes the temporary env file on exit. API/Worker JSON loggers now receive configured DB URL/password, bearer tokens, cursor key, S3 access/secret keys, and provider API key as in-process redaction values. Sensitive attribute names, bearer strings, credential-bearing URLs, common `key=value` forms, and exact configured values are tested locally. This is defense in depth, not proof against arbitrary unregistered secrets, nested opaque values, or a staging log sink; those checks remain outstanding.
- **SSRF:** user-provided task URIs are not used as S3 endpoints. Gateway URL is deployment configuration; production config requires HTTPS. Egress allowlisting/network policy is not implemented.
- **DB least privilege:** a new disposable-database integration test creates a random non-superuser runtime role, applies narrow DML/sequence grants, migrates as the owner, and runs enqueue/claim/fenced ACK plus audited queue pause/resume as the runtime role. The same role is denied DDL, TRUNCATE, hard delete, task-event/audit mutation, and cluster role creation. This is local PostgreSQL role evidence only; no hosted DB role was deployed, no production grant bootstrap is checked in, and task tables still need UPDATE for the repository's state machine.
- **Audit integrity:** privileged DB mutations and audit writes are transactionally tested; external retention/export/immutability controls are not proven.
- **Production fail-closed:** unchanged. `ValidateWorker` still rejects production execution; the new S3 injection path does not itself enable a production task type.

## Release/security decision

No evidence supports enabling a production Worker. A development-profile API/two-Worker deterministic-Eval recovery path is now verified, but stale-generation-after-reclaim, production-approved execution types, deployed least-privilege DB provisioning, real IdP trust, staging telemetry redaction, provider recovery, and operator-reviewed deployment/security posture remain required before revisiting the gate. Local MinIO adapter and local OTLP receiver contracts do not substitute for those production checks.
