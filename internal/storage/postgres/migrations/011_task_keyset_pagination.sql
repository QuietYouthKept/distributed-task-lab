-- Cover stable descending keyset order for global and creator-scoped task lists.
CREATE INDEX IF NOT EXISTS idx_tasks_created_at_id
    ON tasks (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_tasks_creator_created_at_id
    ON tasks (created_by, created_at DESC, id DESC)
    WHERE created_by IS NOT NULL;
