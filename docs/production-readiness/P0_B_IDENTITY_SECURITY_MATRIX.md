# P0-B — Identity and API security test matrix

The local fixture is HTTPS/JWKS/RS256 with test roles; it is not a hosted IdP
and does not provide browser OAuth authorization-code login. Unit/HTTP coverage
does not equal container or PostgreSQL E2E.

| Threat / boundary | Existing coverage | Level / status |
|---|---|---|
| Missing or invalid bearer token | API router authentication and bad-signature tests | In-process HTTP / implemented |
| Expired token, `nbf`, issuer, audience, missing subject | `internal/auth/oidc_test.go` | Unit / covered |
| Algorithm confusion / HS256 | verifier only permits RS256; negative test | Unit / covered |
| Invalid RSA signature / unknown KID | verifier tests | Unit / covered |
| Role claim trust and service identity | role mapping and trusted service client IDs | Unit / covered |
| JWKS key rotation | refreshes for unknown KID | Unit / covered |
| JWKS outage after cache expiry | added bounded fail-closed test | Unit / covered |
| JWKS request timeout | added configured HTTP timeout test | Unit / covered |
| CA trust bundle | test HTTPS server and explicit CA | In-process HTTPS / covered |
| Wrong CA / expired cert / SAN mismatch | not all adversarial cert cases are separately asserted | GAP |
| Reader vs operator/admin mutation | router tests | In-process HTTP with fixture repositories / covered at handler boundary |
| Cross-owner read/cancel/requeue | API and repository tests exist, but full current-snapshot DB matrix not run | PARTIAL; DB E2E unverified |
| Audit authorization and append-only storage | handler boundary plus PG privilege test | DB test not run |
| Token/secret logging | redaction tests exist | Unit / covered; needs live log inspection |
| Full HTTPS API -> PostgreSQL RBAC | Compose E2E | NOT RUN; `RESOURCE_LIMITED` and MinIO source blocked |
| Hosted IdP rotation/operations | no hosted IdP used | EXTERNAL_NOT_VERIFIED |

No production authentication boundary was relaxed. Production execution stays
fail-closed; fixture role mappings are limited to `staging-sim`.
