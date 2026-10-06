# External side-effect adapter

The queue is at-least-once. A worker can apply a payment or notification and
die before its fenced acknowledgement, so the queue cannot make that provider
call exactly-once.

`notification` and `payment` tasks use a small receipt-backed simulator. Its
business idempotency key is `<operation>:<operation_id>`. Before a provider
lookup or apply, the worker stores an `effects` intent containing the trusted
task ID, lease generation, attempt, policy, operation key, and a request hash.
It then looks up the durable receipt, applies only when absent, and treats a
lost response as `unknown`. A retry reconciles the receipt before any apply and
writes its response hash and provider request ID to the same ledger entry.

The effect identity is deterministic for `(task_id, operation_key, generation)`.
A late receipt from an old generation remains audit evidence only: task state
still requires the fencing credential, so it cannot complete a newer owner.
`unknown` effects are operator-visible through
`tasklab_effect_unknown_total`; a real provider adapter without idempotency or
lookup support must use the ledger's `manual_resolution` status instead of
blindly retrying.

Receipt payloads live under `RAG_ARTIFACT_DIR/receipts`. That filesystem is a
simulator boundary, not the authoritative payment ledger: the PostgreSQL
`effects` row is the durable intent/receipt audit record. The supplied payment
example first returns an unknown outcome, then its retry succeeds with
`reconciled=true` and no duplicate effect.

See [`examples/payloads/payment_reconcile.json`](../examples/payloads/payment_reconcile.json).
