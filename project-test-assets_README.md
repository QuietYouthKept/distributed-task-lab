# Project test assets

Correctness experiment cards live in `test-cards/`. Each card records its
invariant, workload, fault, oracle, evidence, seed, current verification state,
and limitations. `[TEST VERIFIED]` means the named automated command actually
passed; `[UNVERIFIED]` is never treated as evidence.

The optional external asset root (`$ASSET_ROOT`) can hold Jepsen/NATS/Redpanda
references during hardening. Repository-owned
cards remain here so CI and reviewers see the same version as the code. External
fixtures/results may link back to a card ID but must not overwrite its oracle.

Run PostgreSQL cards with:

```bash
REQUIRE_TEST_DATABASE=1 go test ./internal/storage/postgres -count=1 -v
```

Run worker lifecycle cards with `go test ./internal/worker -count=1 -v`, and
scenario cards with `make demo-failure-lab`.
