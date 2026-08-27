# TL-LEASE-003 — Renewal then stopped-renewal takeover

- Status: generated, executable evidence card
- Source model: Jepsen invocation/completion histories plus NATS/Redpanda final recovery oracles
- Seed: `20260823`
- Prefix: `asset-20260823-05`
- Oracle: `result.json` assertions; raw operations are in `operations.jsonl`
- Evidence boundary: this experiment proves the stated PostgreSQL queue invariant only; it does not prove end-to-end exactly-once effects or replicated-database durability.
