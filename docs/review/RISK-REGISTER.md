# Risk Register

| Area | Risk | Status | Treatment |
|---|---|---|---|
| Correctness | External side effect duplicated after effect-before-ack crash | Accepted | at-least-once semantics; require business idempotency key/outbox |
| Availability | A constantly contended queue can be skipped | Accepted | prevents cross-queue blocking; document priority/liveness trade-off |
| Database | PostgreSQL HA/failover has not been validated | Future | single PostgreSQL authority only |
| Operations | No long soak, disk exhaustion, corruption, or power-loss test | Future | bounded fault evidence only |
| Security | Bearer authentication is not TLS | Mitigated | production requires TLS termination and secure network |
| Performance | No production sizing claim | Mitigated | publish only bounded benchmark methodology/results |
| Evidence | Historical result files could be hand-edited | Mitigated | checker v2 recomputes key oracles from histories/DB snapshots; artifacts are not signed |
