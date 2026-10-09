# Wave 2 security review

## Reviewed boundaries

- **JWT/JWKS, RBAC, IDOR:** code uses verifier-created principals and explicit claim mappings; resource ownership, static-token restrictions in production config and audit transaction tests are covered locally. External IdP/JWKS rotation was not exercised.
- **Artifact references:** object keys are normalized relative keys; S3 endpoint/bucket/prefix/credentials come from server config, not task payload. Logical keys no longer expose configured prefix. S3 requires TLS except explicit development HTTP.
- **Artifact access:** S3 contract and bad-credential/permission-denial integration were not run in this process. Do not infer access-control proof from a health endpoint.
- **Secret leakage:** test fixture uses placeholder credentials only; this execution did not print environment values. A comprehensive structured-log redaction test across DB, JWT, S3 and provider secrets remains outstanding.
- **SSRF:** user-provided task URIs are not used as S3 endpoints. Gateway URL is deployment configuration; production config requires HTTPS. Egress allowlisting/network policy is not implemented.
- **DB least privilege:** migrations and app queries were exercised with the local test account, not a restricted runtime DB role.
- **Audit integrity:** privileged DB mutations and audit writes are transactionally tested; external retention/export/immutability controls are not proven.
- **Production fail-closed:** unchanged. `ValidateWorker` still rejects production execution; the new S3 injection path does not itself enable a production task type.

## Release/security decision

No evidence supports enabling a production Worker. Require authenticated S3 integration, two-worker fencing/recovery, least-privilege DB, real IdP trust, telemetry redaction, and an operator-reviewed deployment/security posture before revisiting the gate.
