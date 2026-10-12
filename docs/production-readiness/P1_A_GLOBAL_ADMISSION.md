# P1-A Shared Global Admission Status

Date: 2026-10-11 (Asia/Shanghai)

## Existing implementation boundary

The repository contains a PostgreSQL-backed shared ingress limiter and a
bounded fixed-slot bucket table (Migration 012). API wiring is opt-in; the
feature remains disabled unless explicit rate, burst, failure mode, and HMAC
key settings are configured. The principal identity is hashed before storage.
No default policy is being enabled by this local-first work.

## Local verification currently available

Router-level unit tests cover fail-closed behavior on admission-store errors,
explicit fail-open behavior, sanitized errors, 429, and `Retry-After`. Config
tests cover opt-in defaults and require explicit failure policy and key material.
These tests use an in-process admission stub; they do not prove shared database
coordination.

The PostgreSQL integration test
`internal/storage/postgres/shared_ingress_limiter_integration_test.go` creates
two independent limiter instances, races 80 requests against one principal,
checks the shared burst boundary, verifies a second principal is independent,
and checks that raw principal text was not stored. That test has not been run
against a database provisioned exclusively for this worktree in the current
source increment; do not treat historical Wave 4 evidence as acceptance of the
current source SHA.

## Required next acceptance

After the container resource gate is satisfied, provision an isolated temporary
PostgreSQL instance and run the integration test with `REQUIRE_TEST_DATABASE=1`
and an explicit `TEST_DATABASE_URL`. Add database-unavailable recovery and
multi-API process tests before declaring the feature fully verified. Preserve
fail-closed as the safe default until product quota and outage policy are
approved.

Status: `P1_A_LOCAL_IMPLEMENTED: YES`; `P1_A_LOCAL_VERIFIED: UNIT_ONLY`.
Shared-database behavior on the current source SHA remains `NOT RUN`.
`PRODUCTION_NOT_APPROVED` remains unchanged.
