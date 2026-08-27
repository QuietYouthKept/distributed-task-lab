# Handler idempotency

The queue guarantees fenced database acknowledgement, not exactly-once business
effects. Use a stable business operation key (task ID or domain-specific key) at
the external effect boundary.

Preferred patterns:

- unique-key insert/upsert in the target database;
- provider idempotency header persisted before retry;
- inbox/outbox transaction around local state;
- compare-and-set state transition keyed by operation ID.

Do not generate a new effect key per attempt. Record the provider response so a
retry can return the prior outcome. If the provider has no idempotency facility,
accept and monitor possible duplicates or add a durable effect ledger.
