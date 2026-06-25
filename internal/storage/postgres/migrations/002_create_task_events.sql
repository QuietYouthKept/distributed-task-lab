-- 002_create_task_events.sql
-- Task event audit log for distributed-task-lab Phase 6.

CREATE TABLE IF NOT EXISTS task_events (
    id          SERIAL PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    worker_id   TEXT,
    message     TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_events_task_id
    ON task_events (task_id);

CREATE INDEX IF NOT EXISTS idx_task_events_type
    ON task_events (event_type);

CREATE INDEX IF NOT EXISTS idx_task_events_created_at
    ON task_events (created_at);
