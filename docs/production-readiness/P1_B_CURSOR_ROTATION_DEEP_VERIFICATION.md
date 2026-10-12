# P1-B — Cursor key rotation deep verification

Current unit coverage exercises active/previous-key overlap and cutoff,
legacy-v1 cutoff, expiration, future timestamp rejection, owner/filter/page
size binding, tamper rejection, unknown key ID, invalid/ambiguous keyring
configuration, and concurrent decoding. Two independent codec instances with
the same configuration are tested in-process; they are not two API processes.

Still required before operational acceptance: prove secret distribution across
API replicas, staged rotation with old+new overlap, partial rollout behavior,
rollback window, and actual cursor continuation through two live API instances.
No signing secret is written into evidence. Cursor rotation does not justify
weakening owner/filter binding or accepting an unknown key identifier.

Status: UNIT_IMPLEMENTED; MULTI_INSTANCE_E2E_NOT_VERIFIED.
