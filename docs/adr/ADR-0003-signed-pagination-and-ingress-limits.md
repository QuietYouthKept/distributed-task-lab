# ADR-0003: Signed keyset pagination and API ingress limiting

- Status: Accepted for Phase 1 implementation
- Date: 2026-10-09
- Scope: TASK-011, TASK-018

## Decision

The stable `/v1/tasks` list endpoint uses keyset order `(created_at DESC, id
DESC)` with a maximum page size of 100. Its versioned HMAC cursor is bound to
the normalized status/type/queue filters, requested page size, and verified
principal subject. Production requires a deployment-managed signing key of at
least 32 bytes. The unversioned compatibility route retains its existing
array response.

An API ingress token bucket is keyed only by a verified principal (not by
caller-provided identity headers). It is process-local and deliberately
described as such; horizontal deployments need a shared limiter before making
cluster-wide rate claims. Buckets are memory bounded and the endpoint returns
429 with `Retry-After` and the normal request ID envelope.

## Consequences

- Cursor tampering, filter changes, page-size changes, or principal changes
  invalidate continuation tokens.
- Production key rotation invalidates outstanding cursors; operational rollout
  should account for this or later add a key-version ring.
- The limiter is an overload guard per API process, not a tenant-wide quota.
