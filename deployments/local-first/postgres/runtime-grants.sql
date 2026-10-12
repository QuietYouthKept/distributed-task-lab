-- Executed after migrations using the non-superuser migration owner.
-- Runtime roles receive DML only; they do not own tables or have CREATE.
GRANT SELECT ON TABLE schema_migrations TO tasklab_api, tasklab_worker;

GRANT SELECT, INSERT, UPDATE ON TABLE tasks, queues TO tasklab_api;
GRANT SELECT, INSERT ON TABLE task_events TO tasklab_api;
GRANT SELECT, INSERT, UPDATE ON TABLE effects TO tasklab_api;
GRANT SELECT, INSERT ON TABLE security_audit_events TO tasklab_api;
GRANT SELECT, INSERT, UPDATE ON TABLE api_ingress_rate_limit_buckets TO tasklab_api;

GRANT SELECT ON TABLE tasks TO tasklab_worker;
GRANT SELECT ON TABLE task_events TO tasklab_worker;
GRANT SELECT ON TABLE queues TO tasklab_worker;
-- Workers only persist shared token-bucket state. Queue governance (pause,
-- concurrency, rate, burst, and aging policy) is API/operator-owned.
REVOKE UPDATE ON TABLE queues FROM tasklab_worker;
GRANT UPDATE (rate_tokens, rate_updated_at) ON TABLE queues TO tasklab_worker;
GRANT SELECT, INSERT, UPDATE ON TABLE workers TO tasklab_worker;
GRANT SELECT, INSERT, UPDATE ON TABLE effects TO tasklab_worker;

-- Task state transitions are exposed only through routines that bind the
-- logical Worker identity to PostgreSQL session_user. In particular, the
-- Worker role has no direct UPDATE privilege on tasks.
REVOKE UPDATE ON TABLE tasks FROM tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_identity() TO tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_terminalize_exhausted_leases() TO tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_claim_task(TEXT, BIGINT) TO tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_ack(TEXT, INTEGER, JSONB) TO tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_fail(TEXT, INTEGER, TEXT, TEXT, BOOLEAN) TO tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_renew(TEXT, INTEGER, BIGINT) TO tasklab_worker;
GRANT EXECUTE ON FUNCTION tasklab_worker_insert_event(TEXT, TEXT, TEXT, JSONB) TO tasklab_worker;

-- Each execution is bound to the dedicated login used by Worker A. For an
-- additional Worker, provision a separate login and an explicit mapping during
-- the same maintenance window; never share one login across Worker instances.
INSERT INTO worker_db_identities(login_role, worker_id)
VALUES ('tasklab_worker_a', :'worker_id')
ON CONFLICT (login_role) DO UPDATE SET worker_id = EXCLUDED.worker_id;

GRANT USAGE, SELECT ON SEQUENCE task_events_id_seq TO tasklab_api;
GRANT USAGE, SELECT ON SEQUENCE security_audit_events_id_seq TO tasklab_api;
