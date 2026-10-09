# ADR-0004: Operator effect reconciliation

- Status: Accepted for Phase 1 implementation; no real provider is configured
- Date: 2026-10-09
- Scope: TASK-016, TASK-017, TASK-024

## Decision

Unresolved external effects (`intent` or `unknown`) can be listed by operators.
Manual resolution is an explicit admin-only action requiring a bounded reason,
verified principal, and request ID. Updating the effect to
`manual_resolution` and inserting the security audit row happen in one
PostgreSQL transaction. Audit failure rolls the effect state back. The action
records an operator decision; it does not assert that a payment or notification
was actually applied by a provider.

## Consequences

- The durable ledger supports operational review and auditable human decisions.
- Reconciliation authority still depends on provider-specific lookup and
  idempotency APIs. Local simulated receipts are not production providers.
- Manual resolution is irreversible through this endpoint; a correction needs
  a new audited decision or provider-specific reconciliation workflow.
