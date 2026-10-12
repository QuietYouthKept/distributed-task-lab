# P0-B Local Identity Fixture Status

Date: 2026-10-11 (Asia/Shanghai)

## Identity boundary

`cmd/oidc-fixture` is a local HTTPS JWKS and JWT test issuer. It implements the
RS256/JWKS verification subset consumed by the API; it does **not** implement
OIDC discovery, browser login, authorization-code flow, refresh tokens, user
provisioning, or a hosted IdP control plane. It is not suitable for production.

The issuer defaults to `https://identity:8443`, requires a non-empty audience,
and refuses non-HTTPS issuer URLs. The JWKS endpoint serves RSA signing keys
with `kid`; the signing key is generated in process memory. Optional test token
issuance is disabled unless `OIDC_FIXTURE_ENABLE_TEST_ISSUER=true`, and only
fixed reader/operator/admin identities can be minted. The optional test rotate
endpoint is also absent unless issuance is explicitly enabled. Test tokens are
bounded to 15 minutes. Rotation publishes the new key and one previous key; it
does not model a full key revocation/expiry policy.

`scripts/local-first/new-test-certs.ps1` creates a fresh, per-run E: directory,
restricts the directory ACL to the current Windows identity, and generates a
short-lived test CA plus distinct `identity` and `api` leaf certificates with
matching DNS SANs. It refuses to reuse an existing directory. These are test
certificates only and must never be copied into a production deployment.

## Verified locally

- The fixture was tested through `httptest.NewTLSServer` using its certificate
  as the explicit CA bundle; TLS verification remained enabled.
- API `auth.Verifier` accepted the fixed operator and reader identities,
  validated RS256 issuer/audience/expiry through the HTTPS JWKS endpoint, and
  refreshed trust when a new key ID appeared.
- A previous key remained verifiable while published; a tampered token was
  rejected. Test-token issuance is opt-in and role names are fixed.
- Certificate provisioning succeeded after explicitly supplying the local
  OpenSSL config; inspection confirmed `DNS:identity` and `DNS:api` SANs.
- The first certificate-script attempt failed because the bundled OpenSSL
  referenced a missing build-time default config. Its partial directory was
  retained and its ACL restricted; the script was fixed to provide its own
  config and a fresh-directory attempt succeeded.

The fixture unit-test log is under
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\oidc-fixture-20261011T131200+0800`.
Generated test TLS material is outside Git at
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\secrets\oidc-fixture-20261011T1313`.

## Remaining acceptance gaps

- No Compose service has run; API-to-fixture verification in separate
  containers remains pending the 6 GiB container resource gate.
- No full OIDC browser login/discovery/refresh flow is implemented.
- No API HTTP matrix has yet been run against this fixture for reader/operator,
  owner isolation, admin routes, expired/wrong issuer/wrong audience tokens, or
  JWKS outage/CA mismatch.
- Real external IdP operations remain `NOT VERIFIED`.

Status: `P0_B_LOCAL_IMPLEMENTED: PARTIAL`; `P0_B_LOCAL_VERIFIED: FIXTURE_UNIT_AND_TLS_TEST_ONLY`.
This does not imply full OIDC or production identity readiness.
`PRODUCTION_NOT_APPROVED` remains unchanged.
