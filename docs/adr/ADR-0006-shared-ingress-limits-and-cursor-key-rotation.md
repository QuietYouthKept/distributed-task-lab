# ADR-0006: Shared ingress quota and cursor signing-key rotation

- Status: Proposed; no cluster-wide quota or key-ring behavior is enabled.
- Date: 2026-10-10.

## Context

The API ingress limiter is a process-local token bucket keyed by the verified Principal subject. It is an overload guard for one process, not a global user quota. It emits 429 with `Retry-After`, and its bucket count is bounded. Wave 3 adds safe reclamation only when an inactive bucket would already have fully refilled; this prevents a saturated 10,000-subject map from permanently rejecting new identities without changing bucket refill semantics.

The task-list cursor is a stateless HMAC token bound to the subject, normalized query filters, page size, and keyset position. It currently has one configured signing key and no explicit expiry. Replacing that key immediately invalidates all outstanding cursors. Cursor replay is currently possible and is expected for pagination; listing is read-only and does not itself cause a side effect.

## Decision proposal — rate limiting

Keep the existing per-process limiter as an explicitly named local overload guard. Do not describe it as a cluster quota. Do not enable a global quota until the owner chooses the following semantics:

1. Quota identity and scope: per subject, tenant, API key, endpoint class, or a combination; define whether burst tokens are shared across all API replicas.
2. Shared state: PostgreSQL gives durable, inspectable state but adds write contention and DB coupling to each request; Redis offers low-latency atomic scripts but adds an operational dependency and eviction/failover semantics; an API gateway centralizes admission but changes the trusted identity boundary and deployment dependency.
3. Backend outage: fail-open preserves availability but loses global enforcement; fail-closed preserves quota but can turn a dependency incident into API outage. The choice must be explicit per use case.
4. Layering: local limiter may reject early for overload even when a shared quota permits the call; `Retry-After` must identify the delay relevant to the rejecting layer.

Candidate verification before enabling a shared mode: two independent API instances; same/different subjects; burst/refill; concurrent atomic admission; backend outage and recovery; bounded state; Retry-After; and measured P95/DB or Redis contention. Record the benchmark environment and do not promote the local result to an SLO.

## Decision proposal — cursor key rotation

Do not change the current version-1 token or acceptance policy in this change. A future opt-in key-ring format should carry an explicit key ID and signed `issued_at`/`expires_at`, bind every token to Principal/filter/page size as today, and accept a historical key only through the earlier of its configured `accept_until` and the cursor expiry. The key set should identify one issuing key and bounded verification-only keys. Unknown key IDs, expired tokens, tampering, principal/filter/page-size changes must fail closed.

Stateless list-cursor replay remains allowed within the TTL; one-time replay protection would require server-side state and is not justified for a read-only pagination cursor. Version-1 migration must be an explicit choice: either immediate invalidation when rotation is enabled or a configured, finite legacy-key overlap. No historical key may be accepted indefinitely. Rotation overlap and TTL are configuration/owner decisions, not assumed defaults.

Before implementation, decide maximum cursor lifetime, rotation overlap, rollout order across API replicas, secret-distribution source, and behavior for clients presenting an old cursor after the overlap. Tests must cover current-key issuance, old-key overlap acceptance/rejection, unknown key, expiry, tampering, principal/filter/page-size binding, concurrent rotation, and explicit version-1 compatibility.

## Consequences and status

- Wave 3 only fixes local bucket reclamation after full refill; shared quota semantics and production configuration are unchanged.
- Cursor version 1 and one-key production configuration remain unchanged. Rotation is not implemented or verified.
- `EXTERNAL_IDP_NOT_VERIFIED` and production identity key distribution remain separate gates.
- A production rollout requires an accepted decision and corresponding integration/operational evidence; this ADR alone is not implementation evidence.
