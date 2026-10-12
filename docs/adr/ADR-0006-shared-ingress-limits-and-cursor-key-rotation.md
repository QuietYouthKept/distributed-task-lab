# ADR-0006: Shared ingress quota and cursor signing-key rotation

- Status: Proposed; implementation is an opt-in candidate. No cluster-wide quota or key-ring behavior is enabled by default, and no product policy approval is implied.
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

Implemented candidate: optional PostgreSQL token bucket with a fixed 10,000-slot table, HMAC-SHA256 principal digests (raw subject is not stored), database `statement_timestamp()` for refill, row locking/`SKIP LOCKED`, and local process limiter retained as an overload guard. When all slots are occupied by partially depleted buckets, admission is denied with a one-second retry. Shared-store errors are explicitly configured fail-open or fail-closed; fail-open still keeps the local overload limiter. The mode is disabled by default. A two-container local E2E verified a shared burst of 16 across two independently built API containers (16 admitted, 48 rate-limited under concurrent requests). A separate PostgreSQL integration test verifies independent limiter objects, principal isolation, 10,000 bounded rows, and no raw subject storage. Backend outage/recovery and production contention SLOs remain unverified.

## Decision proposal — cursor key rotation

Do not change the current version-1 token or acceptance policy by default. An opt-in version-2 key-ring candidate now carries a key ID and signed `issued_at`/`expires_at`, binds every token to Principal/filter/page size, and accepts a historical key only through its configured `accept_until` and the token expiry. The key set identifies one issuing key and bounded verification-only keys. Unknown key IDs, expired tokens, tampering, principal/filter/page-size changes fail closed. HMAC comparison uses constant-time verification. The default constructor continues to issue and accept the compatible v1 cursor format as before.

Stateless list-cursor replay remains allowed within the TTL; one-time replay protection would require server-side state and is not justified for a read-only pagination cursor. Version-1 migration must be an explicit choice: either immediate invalidation when rotation is enabled or a configured, finite legacy-key overlap. No historical key may be accepted indefinitely. Rotation overlap and TTL are configuration/owner decisions, not assumed defaults.

The candidate validates a maximum seven-day TTL and finite `accept_until` boundaries. Tests cover current-key issuance, old-key overlap acceptance/rejection, unknown key, expiry, tampering, principal/filter/page-size binding, concurrent decode, and explicit v1 compatibility cutoff. Before enabling it, decide TTL/overlap, rollout order across replicas, secret-distribution source, and client behavior after overlap. A configured finite v1 legacy acceptance window is required if old v1 cursors must survive opt-in; otherwise enabling the candidate intentionally rejects v1 tokens. No default was changed.

## Consequences and status

- The shared PostgreSQL candidate and cursor key ring are implemented and locally tested, but both remain explicitly opt-in. The product quota/failure policy has not been approved.
- Production configuration remains unchanged; cursor v1 remains the default. The key ring has unit/Race compatibility-boundary tests, but production secret distribution and a rolling multi-version deployment have not been exercised.
- Candidate source and evidence are documented in `docs/wave4/WAVE4_PACKAGE_D_GLOBAL_ADMISSION.md`; local run evidence is stored under `E:\\distributed-task-lab-temp\\wave4-artifacts\\shared-admission-e2e-runs`.
- `EXTERNAL_IDP_NOT_VERIFIED` and production identity key distribution remain separate gates.
- A production rollout requires an accepted decision and corresponding integration/operational evidence; this ADR alone is not implementation evidence.
