# External side-effect adapter

The queue is at-least-once. A worker can apply a payment or notification and
die before its fenced acknowledgement, so the queue cannot make that provider
call exactly-once.

`notification` and `payment` tasks use a small receipt-backed
simulator. Its business idempotency key is `<operation>:<operation_id>`. The
adapter first looks up the durable receipt, applies only when absent, and treats
a lost response as unknown. A retry reconciles the receipt before any apply.

Receipts live under `RAG_ARTIFACT_DIR/receipts`. Mount that directory on durable
storage in a deployment; it is a simulator boundary, not a claim that a local
container filesystem is a production payment ledger. The supplied payment
example first returns an unknown outcome, then its retry succeeds with
`reconciled=true` and no duplicate effect.

See [`examples/payloads/payment_reconcile.json`](../examples/payloads/payment_reconcile.json).
