-- 001_create_tasks.sql
-- Initial tasks table for distributed-task-lab Phase 1.

CREATE TABLE IF NOT EXISTS tasks (
    id                  TEXT PRIMARY KEY,
    type                TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    payload             JSONB NOT NULL DEFAULT '{}',
    result              JSONB,
    priority            INTEGER NOT NULL DEFAULT 0,
    max_attempts        INTEGER NOT NULL DEFAULT 3,
    attempt             INTEGER NOT NULL DEFAULT 0,
    timeout_seconds     INTEGER NOT NULL DEFAULT 60,
    retry_delay_seconds INTEGER NOT NULL DEFAULT 5,
    leased_by           TEXT,
    lease_expires_at    TIMESTAMPTZ,
    next_run_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    last_error          TEXT,
    dlq_reason          TEXT,
    idempotency_key     TEXT
);

-- Index for worker claim queries (used in Phase 2+).
CREATE INDEX IF NOT EXISTS idx_tasks_status_next_run
    ON tasks (status, next_run_at, priority);

-- Index for stale-lease detection (used in Phase 2+).
CREATE INDEX IF NOT EXISTS idx_tasks_status_lease
    ON tasks (status, lease_expires_at);

-- Index for API filtering by type.
CREATE INDEX IF NOT EXISTS idx_tasks_type
    ON tasks (type);

-- Index for time-based queries.
CREATE INDEX IF NOT EXISTS idx_tasks_created_at
    ON tasks (created_at);

-- Index for idempotency key lookups.
CREATE INDEX IF NOT EXISTS idx_tasks_idempotency_key
    ON tasks (idempotency_key);
