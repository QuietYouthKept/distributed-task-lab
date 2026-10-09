# ADR-0001: OIDC identity, single-tenant resource scope, and security audit

- Status: Accepted for Phase 1 implementation
- Date: 2026-10-09
- Scope: TASK-003, TASK-006, TASK-007, TASK-008, TASK-024

## Context

The API previously used optional shared static bearer tokens. The project is a
single-tenant internal task platform for this phase. It needs a trusted
request-scoped human/service identity, explicit role mapping, task-level IDOR
protection, and durable attribution for privileged mutations. Production
workers and task execution remain fail-closed until their provider and shared
artifact paths are independently verified.

## Decisions

1. Production API authentication uses JWTs from one configured OIDC issuer,
   audience, and fixed HTTPS JWKS endpoint. Only RS256/RSA keys are accepted.
   `exp` is required; issuer, audience, signature, and `nbf` when present are
   validated. JWKS fetches are bounded and redirects are rejected. Unknown key
   IDs force refresh; expired cache refresh failures fail closed.
2. Role claims are mapped by explicit configuration to `reader`, `operator`,
   or `admin`. Unknown claim values grant no elevated role. Admin inherits
   operator and reader; operator inherits reader. Service identity requires a
   configured trusted client ID (`azp` or `client_id`); otherwise identity is
   treated as human.
3. Development static tokens remain compatibility-only. Production config
   rejects static token auth and requires complete OIDC configuration.
4. A task is readable/mutable by its trusted `created_by` subject; admin has
   global scope. Legacy rows without `created_by` are hidden from non-admins.
   Caller-supplied owner/actor values are never accepted. Idempotency is scoped
   by creator so separate principals cannot retrieve one another's task by
   reusing a key.
5. Privileged task/queue mutations write an append-only security audit row in
   the same PostgreSQL transaction. Missing actor/request ID or failed audit
   persistence aborts the mutation. Audit rows omit token and payload values.
6. Production task execution currently has an explicit empty allowlist. This
   intentionally rejects even structurally valid mock/deterministic payloads
   until a complete supported handler path is enabled. The worker continues to
   fail closed in production.

## Consequences

- Single-tenant identity is implemented without a cross-organization schema.
- Existing task rows require admin access until an explicit secure backfill is
  designed.
- Production API can authenticate but currently rejects all task execution
  modes. This is safer than silently accepting simulation or claiming a
  production handler that does not exist.
- The database trigger blocks ordinary UPDATE/DELETE statements on audit rows.
  Production still needs a least-privilege runtime DB role that does not own
  the table or migration schema; this deployment hardening is not proven here.

## Verification

Local ephemeral-RSA/JWKS tests cover valid signatures, issuer/audience,
expiration, not-before, forged signatures, disallowed algorithms, trusted role
mapping, and key rotation. PostgreSQL integration tests cover creator scoping,
idempotency isolation, atomic audit writes, and fail-closed behavior if audit
insertion fails. These test outcomes are recorded in
`docs/phase1-build/PROGRESS.md`; they do not imply a staging OIDC provider.
