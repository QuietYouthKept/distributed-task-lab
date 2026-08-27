# Contributing

Keep changes correctness-first and PostgreSQL-focused. Add a regression test for
state-machine, ownership, queue governance, or migration changes. PostgreSQL
locking/MVCC properties require real integration tests, not mocks.

Run `make verify` and `make integration`. Do not edit applied migration files;
add the next numbered forward migration. Update guarantees, failure model, and a
Test Card when changing observable semantics.
