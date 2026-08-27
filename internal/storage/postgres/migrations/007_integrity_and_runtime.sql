-- 007_integrity_and_runtime.sql
-- Referential integrity, authoritative state checks, and cluster-wide queue
-- token-bucket state. Constraints are NOT VALID so an existing installation
-- can upgrade without rewriting or silently changing historical rows; they
-- still protect every new/updated row immediately.

INSERT INTO queues (name)
SELECT DISTINCT queue FROM tasks
ON CONFLICT (name) DO NOTHING;

ALTER TABLE queues ADD COLUMN IF NOT EXISTS rate_tokens DOUBLE PRECISION NOT NULL DEFAULT 1;
ALTER TABLE queues ADD COLUMN IF NOT EXISTS rate_updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();
UPDATE queues SET rate_tokens = LEAST(rate_tokens, burst::double precision);

ALTER TABLE task_events ALTER COLUMN id TYPE BIGINT;

DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_queue_fk
        FOREIGN KEY (queue) REFERENCES queues(name) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_status_check
        CHECK (status IN ('pending','running','success','failed','dlq')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_attempt_check CHECK (attempt >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_max_attempts_check CHECK (max_attempts > 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_timeout_check CHECK (timeout_seconds > 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_retry_delay_check CHECK (retry_delay_seconds >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_retry_max_delay_check CHECK (retry_max_delay_seconds >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_retry_multiplier_check CHECK (retry_backoff_multiplier >= 1) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT tasks_lease_version_check CHECK (lease_version >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_name_check CHECK (length(name) BETWEEN 1 AND 128) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_max_concurrency_check CHECK (max_concurrency >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_rate_check CHECK (rate_limit_per_second >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_burst_check CHECK (burst >= 1) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_rate_tokens_check CHECK (rate_tokens >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    ALTER TABLE workers ADD CONSTRAINT workers_task_count_check CHECK (current_task_count >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE workers ADD CONSTRAINT workers_status_check CHECK (status IN ('active','draining','inactive')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Hot-path indexes correspond to the two claim predicates. Terminal rows do
-- not pay the maintenance cost.
CREATE INDEX IF NOT EXISTS idx_tasks_pending_claim
    ON tasks (queue, priority DESC, next_run_at, created_at)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_tasks_expired_running
    ON tasks (lease_expires_at, queue)
    WHERE status = 'running';
