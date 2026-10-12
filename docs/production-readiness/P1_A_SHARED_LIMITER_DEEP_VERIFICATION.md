# P1-A — Shared ingress limiter deep verification

The limiter is PostgreSQL-backed, disabled by default, hashes principal names
with HMAC, uses PostgreSQL statement time, bounds slot cardinality to 10,000,
and only reclaims a bucket after it would be fully refilled. It can coordinate
separate instances only when both use the same database and hash key.

| Scenario | Existing coverage | Status |
|---|---|---|
| Router binds quota to verified subject | `TestIngressRateLimitIsBoundToVerifiedPrincipal` | in-process unit present |
| Retry-After and failure modes | `TestSharedIngressFailureModesAndRetryAfter` | in-process unit present |
| Slot reclaim only after full refill | `TestIngressLimiterReclaimsOnlyFullyRefilledIdleBuckets` | in-process unit present |
| Multiple limiter instances contend in one PostgreSQL | `TestSharedIngressLimiterCoordinatesIndependentInstancesAndBoundsState` | real PG test exists; not run in this checkpoint |
| DB timeout/unavailable/recovery | DB integration case not exercised here | NOT VERIFIED |
| Hash key rotation / principal cardinality | code/documentation review; no dual-key migration | PARTIAL |
| Pool pressure / fairness | no load test in scope | NOT VERIFIED |

No Mock or in-process test is represented as global-quota acceptance. Production
rate limiting remains opt-in and its failure mode must be an explicit product
decision.
