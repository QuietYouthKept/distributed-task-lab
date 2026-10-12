# ADR-0005: Provider-independent HTTP side-effect contract

- Status: Accepted for contract testing; not approved for production integration.
- Date: 2026-10-10.

## Context

Task delivery is at-least-once. PostgreSQL can persist an effect intent, but it cannot make a remote provider transaction atomic with the task ACK. A transport timeout, reset, or 5xx after request dispatch therefore cannot safely be treated as proof that no external effect occurred. The repository has no owner-approved payment/notification provider, sandbox credentials, webhook verification policy, or reconciliation authority.

## Decision

Add a provider-independent `effects.Provider` port and a small HTTP contract client for isolated protocol and fault-injection tests. Each apply request carries an operation ID, stable idempotency key, request ID, task ID, lease generation, attempt number, operation, target, and payload. Retries must preserve operation/idempotency identity and request content; attempt and generation remain diagnostic execution identity, not a new external operation.

The contract distinguishes `not_sent`, confirmed success, confirmed permanent rejection, explicit retryable rejection, and unknown outcome. Once request bytes may have been sent, a timeout/reset is `unknown`. A 5xx or invalid success receipt is also `unknown`. Only this test contract defines 429 as a rejection safe to retry; an actual provider's documented semantics must override this mapping. Error responses are not echoed to avoid leaking provider data into logs.

Reconciliation is a separate query and may report pending, completed, failed, not found, or unknown. A not-found result is not by itself permission to repeat a non-idempotent operation. The caller must apply provider-specific policy and preserve the ledger evidence.

The HTTP adapter is not wired into production handler selection. The existing file-backed simulator remains development-only, and Production Worker admission remains fail-closed. No production provider is verified.

## Verification

`internal/effects/http_provider_test.go` uses a real local HTTP listener to check success, permanent/retryable rejection, 5xx, malformed response, redirect, timeout after request dispatch, connection reset, duplicate idempotent request, same-key/different-payload conflict, reconciliation states, endpoint validation, and pre-dispatch cancellation. These are contract/fault-injection checks, not provider sandbox certification.

## Consequences

- Provider behavior remains isolated from scheduling state and database transaction semantics.
- Unknown outcomes are explicitly preserved for reconciliation; transport failure does not silently become confirmed failure.
- A future adapter requires a selected provider, approved sandbox, documented idempotency and reconciliation semantics, authentication/signature policy, and a real sandbox E2E before any production admission change.
- Rollback is a code revert of the new adapter and tests; there is no schema migration or runtime configuration change.
