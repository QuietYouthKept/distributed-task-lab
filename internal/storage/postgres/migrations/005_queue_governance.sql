-- 005_queue_governance.sql
-- P3: per-queue concurrency caps and pause/resume governance.

CREATE TABLE IF NOT EXISTS queues (
    name                  TEXT PRIMARY KEY,
    max_concurrency       INTEGER NOT NULL DEFAULT 0,  -- 0 = unlimited
    rate_limit_per_second REAL NOT NULL DEFAULT 0,      -- 0 = unlimited (reserved)
    paused                BOOLEAN NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO queues (name) VALUES ('default')
ON CONFLICT (name) DO NOTHING;

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS queue TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_tasks_queue_status
    ON tasks (queue, status, next_run_at);
