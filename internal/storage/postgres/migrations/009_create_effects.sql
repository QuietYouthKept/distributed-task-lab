-- Durable evidence for effects outside the task database. This table is not
-- cascaded from tasks: audit/reconciliation evidence must survive task cleanup.
CREATE TABLE IF NOT EXISTS effects (
    effect_id UUID PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    operation_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    policy TEXT NOT NULL CHECK (policy IN ('pure', 'idempotent', 'provider-idempotent', 'reconcilable', 'non-retriable')),
    status TEXT NOT NULL CHECK (status IN ('intent', 'applied', 'unknown', 'manual_resolution')),
    provider_request_id TEXT,
    request_hash TEXT NOT NULL,
    response_hash TEXT,
    generation INTEGER NOT NULL CHECK (generation > 0),
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    resolution_note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    reconciled_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_effects_task_operation_generation
    ON effects(task_id, operation_key, generation);
CREATE INDEX IF NOT EXISTS idx_effects_reconciliation
    ON effects(status, created_at) WHERE status IN ('intent', 'unknown', 'manual_resolution');
