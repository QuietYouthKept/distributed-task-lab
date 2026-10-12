-- Executed after migrations using the non-superuser migration owner.
-- Runtime roles receive DML only; they do not own tables or have CREATE.
GRANT SELECT ON TABLE schema_migrations TO tasklab_api, tasklab_worker;

GRANT SELECT, INSERT, UPDATE ON TABLE tasks, queues TO tasklab_api;
GRANT SELECT, INSERT ON TABLE task_events TO tasklab_api;
GRANT SELECT, INSERT, UPDATE ON TABLE effects TO tasklab_api;
GRANT SELECT, INSERT ON TABLE security_audit_events TO tasklab_api;
GRANT SELECT, INSERT, UPDATE ON TABLE api_ingress_rate_limit_buckets TO tasklab_api;

GRANT SELECT, UPDATE ON TABLE tasks TO tasklab_worker;
GRANT SELECT, INSERT ON TABLE task_events TO tasklab_worker;
GRANT SELECT ON TABLE queues TO tasklab_worker;
-- Workers only persist shared token-bucket state. Queue governance (pause,
-- concurrency, rate, burst, and aging policy) is API/operator-owned.
REVOKE UPDATE ON TABLE queues FROM tasklab_worker;
GRANT UPDATE (rate_tokens, rate_updated_at) ON TABLE queues TO tasklab_worker;
GRANT SELECT, INSERT, UPDATE ON TABLE workers TO tasklab_worker;
GRANT SELECT, INSERT, UPDATE ON TABLE effects TO tasklab_worker;

GRANT USAGE, SELECT ON SEQUENCE task_events_id_seq TO tasklab_api, tasklab_worker;
GRANT USAGE, SELECT ON SEQUENCE security_audit_events_id_seq TO tasklab_api;
