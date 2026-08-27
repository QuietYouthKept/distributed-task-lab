# Security policy

Report vulnerabilities privately through GitHub Security Advisories. Do not
open a public issue containing credentials, exploit details, or sensitive data.

Production deployments must configure TLS at ingress, `API_TOKEN`, a distinct
`ADMIN_TOKEN`, PostgreSQL least privilege/network isolation, request-size limits,
and platform-managed secrets. The built-in bearer boundary is intentionally
small; it is not enterprise IAM. Rotate a token by rolling instances with the
new value. Never place payload contents or tokens in telemetry.
