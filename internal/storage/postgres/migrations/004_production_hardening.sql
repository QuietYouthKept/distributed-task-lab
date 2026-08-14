-- 004_production_hardening.sql
-- P1: idempotency (unique key), fencing token (lease_version), retry backoff.

-- Replace the non-unique idempotency index with a unique one. A plain UNIQUE
-- index still allows multiple NULL keys, so tasks without an idempotency_key
-- are unaffected.
DROP INDEX IF EXISTS idx_tasks_idempotency_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_idempotency_key ON tasks (idempotency_key);

-- Fencing token: incremented on every claim so a stale worker from an earlier
-- lease generation cannot write back results.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS lease_version INTEGER NOT NULL DEFAULT 0;

-- Retry backoff configuration (exponential backoff + jitter).
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS retry_max_delay_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS retry_backoff_multiplier DOUBLE PRECISION NOT NULL DEFAULT 2.0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS retry_jitter BOOLEAN NOT NULL DEFAULT true;
