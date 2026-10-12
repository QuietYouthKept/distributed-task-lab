# P1-B Cursor Key Rotation Status

Date: 2026-10-11 (Asia/Shanghai)

## Implemented contract

The rotating cursor format uses a key ID, issued-at and expiry timestamps, and
an HMAC. The active key signs new cursors. Explicit previous keys verify old
cursors only through their `AcceptUntil` bound. Optional legacy v1 acceptance
also requires a finite cutoff. Decoding binds the cursor to the query filter,
page size, and authenticated principal. Unknown/removed key IDs, invalid MACs,
expired tokens, and issuance beyond the accepted clock skew are rejected.
Rotation remains opt-in; the compatibility constructor is unchanged.

## Local unit verification

On the follow-up implementation tree, focused tests passed under Go 1.26.9:

```text
go test ./internal/api/handlers ./internal/config -run 'Cursor' -count=1 -v
```

The tests cover active/previous-key overlap, cutoff expiry, legacy v1 cutoff,
tampering, unknown key IDs, expired cursors, filter/page-size/principal binding,
future issuance, concurrent verification, invalid keyring configuration, and
opt-in configuration defaults. A second codec instance with the same keyring
successfully validates an active-key cursor, which is a unit-level consistency
check only.

Evidence directory:
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\cursor-rotation-20261011T130700+0800`.
The source commit/tree and final test log are recorded in its manifest after the
verified commit is created.

## Not verified / rollout constraints

- No two running API processes were rotated under load; container E2E was not
  run because available RAM was below the 6 GiB container gate.
- No automated key distribution or secret-file rollout controller exists here.
- Operators must deploy a ring containing the new active key plus the previous
  verification key to every API instance before/while switching signers; retain
  the old key through the maximum cursor TTL and explicit overlap, then remove
  it only after the cutoff. Partial or stale instance configuration can cause
  intermittent 400 responses and is not made safe by the unit test.
- This is not production key-management, KMS, or hosted secret-rotation evidence.

Status: `P1_B_LOCAL_IMPLEMENTED: PARTIAL`; `P1_B_LOCAL_VERIFIED: UNIT_ONLY`.
Production key distribution remains unverified and `PRODUCTION_NOT_APPROVED`.
