ALTER TABLE tasks ADD COLUMN IF NOT EXISTS created_by TEXT;

-- Idempotency is scoped to the trusted task creator. Historical rows are
-- intentionally left ownerless and are visible only to administrators.
DROP INDEX IF EXISTS idx_tasks_idempotency_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_owner_idempotency
    ON tasks (created_by, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND created_by IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_legacy_idempotency
    ON tasks (idempotency_key)
    WHERE idempotency_key IS NOT NULL AND created_by IS NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_owner_created
    ON tasks (created_by, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS security_audit_events (
    id BIGSERIAL PRIMARY KEY,
    actor_id TEXT NOT NULL,
    actor_kind TEXT NOT NULL CHECK (actor_kind IN ('human', 'service')),
    request_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
    reason TEXT NOT NULL DEFAULT '',
    context JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS idx_security_audit_resource_time
    ON security_audit_events (resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_security_audit_actor_time
    ON security_audit_events (actor_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_security_audit_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'security audit events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS security_audit_append_only ON security_audit_events;
CREATE TRIGGER security_audit_append_only
    BEFORE UPDATE OR DELETE ON security_audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_security_audit_mutation();
