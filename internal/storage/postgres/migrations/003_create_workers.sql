-- 003_create_workers.sql
-- Worker registry and heartbeat for distributed-task-lab Phase 6.

CREATE TABLE IF NOT EXISTS workers (
    worker_id          TEXT PRIMARY KEY,
    hostname           TEXT,
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    current_task_count INTEGER NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'active',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workers_last_seen
    ON workers (last_seen_at);
