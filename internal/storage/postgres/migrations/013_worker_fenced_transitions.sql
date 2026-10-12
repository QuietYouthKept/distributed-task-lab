-- Worker state changes are performed by narrowly scoped SECURITY DEFINER
-- routines. The database login (session_user), not a caller-supplied
-- worker_id, is the authority used for the fencing predicate.
CREATE TABLE IF NOT EXISTS worker_db_identities (
    login_role NAME PRIMARY KEY,
    worker_id  TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT worker_db_identities_role_name_check
        CHECK (login_role::TEXT ~ '^[a-z_][a-z0-9_]{0,62}$'),
    CONSTRAINT worker_db_identities_worker_id_check
        CHECK (length(worker_id) BETWEEN 1 AND 256)
);

REVOKE ALL ON TABLE worker_db_identities FROM PUBLIC;

CREATE OR REPLACE FUNCTION tasklab_worker_identity()
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    mapped_worker_id TEXT;
BEGIN
    SELECT i.worker_id INTO mapped_worker_id
    FROM public.worker_db_identities AS i
    WHERE i.login_role = session_user::NAME;
    IF mapped_worker_id IS NULL THEN
        RAISE EXCEPTION 'database login is not mapped to a Worker identity'
            USING ERRCODE = '42501';
    END IF;
    RETURN mapped_worker_id;
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_terminalize_exhausted_leases()
RETURNS TABLE(task_id TEXT, worker_id TEXT)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT := public.tasklab_worker_identity();
    queue_row RECORD;
BEGIN
    -- Use the same queue-before-task lock order as normal claims.
    PERFORM set_config('tasklab.failure_type', 'worker_crash', true);
    FOR queue_row IN
        SELECT DISTINCT t.queue
          FROM public.tasks AS t
         WHERE t.status = 'running'
           AND t.lease_expires_at < clock_timestamp()
           AND t.attempt >= t.max_attempts
         ORDER BY t.queue
    LOOP
        PERFORM 1 FROM public.queues AS q WHERE q.name = queue_row.queue FOR UPDATE;
        RETURN QUERY
        UPDATE public.tasks AS t
           SET status = 'dlq',
               dlq_reason = 'max attempts exceeded after worker lease expiry',
               last_error = 'worker lease expired',
               finished_at = clock_timestamp(),
               updated_at = clock_timestamp(),
               leased_by = NULL,
               lease_expires_at = NULL
         WHERE t.queue = queue_row.queue
           AND t.status = 'running'
           AND t.lease_expires_at < clock_timestamp()
           AND t.attempt >= t.max_attempts
        RETURNING t.id, caller_worker_id;
    END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_claim_task(p_task_id TEXT, p_lease_timeout_us BIGINT)
RETURNS TABLE(lease_expires_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
              attempt INTEGER, lease_version INTEGER, started_at TIMESTAMPTZ,
              was_reclaim BOOLEAN, worker_id TEXT)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT := public.tasklab_worker_identity();
    reclaiming BOOLEAN;
    queue_rate DOUBLE PRECISION;
    queue_burst INTEGER;
    queue_tokens DOUBLE PRECISION;
    queue_updated_at TIMESTAMPTZ;
    queue_paused BOOLEAN;
    queue_max_concurrency INTEGER;
    queue_name TEXT;
BEGIN
    IF p_lease_timeout_us <= 0 OR p_lease_timeout_us > 86400000000 THEN
        RAISE EXCEPTION 'invalid lease duration' USING ERRCODE = '22023';
    END IF;
    SELECT t.queue INTO queue_name FROM public.tasks AS t WHERE t.id = p_task_id;
    IF NOT FOUND THEN
        RETURN;
    END IF;
    SELECT q.rate_limit_per_second, q.burst, q.rate_tokens, q.rate_updated_at,
           q.paused, q.max_concurrency
      INTO queue_rate, queue_burst, queue_tokens, queue_updated_at,
           queue_paused, queue_max_concurrency
      FROM public.queues AS q WHERE q.name = queue_name FOR UPDATE;
    IF NOT FOUND OR queue_paused THEN
        RETURN;
    END IF;
    SELECT (t.status = 'running') INTO reclaiming
      FROM public.tasks AS t
     WHERE t.id = p_task_id
       AND ((t.status = 'pending' AND t.next_run_at <= clock_timestamp())
         OR (t.status = 'running' AND t.lease_expires_at < clock_timestamp()))
     FOR UPDATE;
    IF NOT FOUND THEN
        RETURN;
    END IF;
    IF NOT reclaiming AND queue_max_concurrency > 0 AND
       (SELECT count(*) FROM public.tasks AS active
         WHERE active.queue = queue_name
           AND active.status = 'running') >= queue_max_concurrency THEN
        RETURN;
    END IF;
    IF queue_rate > 0 THEN
        queue_tokens := LEAST(queue_burst::DOUBLE PRECISION,
            queue_tokens + GREATEST(0, EXTRACT(EPOCH FROM (clock_timestamp() - queue_updated_at))::DOUBLE PRECISION) * queue_rate);
        IF queue_tokens < 1 THEN
            RETURN;
        END IF;
        UPDATE public.queues
           SET rate_tokens = queue_tokens - 1, rate_updated_at = clock_timestamp()
         WHERE name = queue_name;
    END IF;
    RETURN QUERY
    UPDATE public.tasks AS t
       SET status = 'running',
           leased_by = caller_worker_id,
           lease_expires_at = clock_timestamp() + (p_lease_timeout_us * interval '1 microsecond'),
           lease_version = t.lease_version + 1,
           started_at = COALESCE(t.started_at, clock_timestamp()),
           updated_at = clock_timestamp(),
           attempt = t.attempt + 1
      FROM public.queues AS q
     WHERE t.id = p_task_id
       AND q.name = t.queue
       AND q.paused = false
       AND (
           (t.status = 'pending' AND t.next_run_at <= clock_timestamp())
           OR (t.status = 'running' AND t.lease_expires_at < clock_timestamp())
       )
    RETURNING t.lease_expires_at, t.updated_at, t.attempt, t.lease_version,
              t.started_at, reclaiming, caller_worker_id;
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_ack(
    p_task_id TEXT, p_lease_version INTEGER, p_result JSONB
)
RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT := public.tasklab_worker_identity();
    changed INTEGER;
BEGIN
    UPDATE public.tasks
       SET status = 'success', result = p_result,
           finished_at = clock_timestamp(), updated_at = clock_timestamp(),
           leased_by = NULL, lease_expires_at = NULL
     WHERE id = p_task_id AND status = 'running'
       AND leased_by = caller_worker_id AND lease_version = p_lease_version;
    GET DIAGNOSTICS changed = ROW_COUNT;
    RETURN changed;
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_fail(
    p_task_id TEXT, p_lease_version INTEGER, p_message TEXT,
    p_error_type TEXT, p_retryable BOOLEAN
)
RETURNS TABLE(final_status TEXT, retry_at TIMESTAMPTZ, attempt INTEGER, worker_id TEXT)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT := public.tasklab_worker_identity();
    t public.tasks%ROWTYPE;
    db_now TIMESTAMPTZ := clock_timestamp();
    delay_seconds NUMERIC;
    delay_us BIGINT;
    dlq_message TEXT;
BEGIN
    SELECT * INTO t FROM public.tasks
     WHERE id = p_task_id AND status = 'running'
       AND leased_by = caller_worker_id AND lease_version = p_lease_version
     FOR UPDATE;
    IF NOT FOUND THEN
        RETURN;
    END IF;
    PERFORM set_config('tasklab.failure_type', COALESCE(p_error_type, ''), true);
    IF NOT p_retryable OR t.attempt >= t.max_attempts THEN
        dlq_message := CASE WHEN p_retryable
            THEN 'max attempts exceeded: ' ELSE 'non-retryable error: ' END || p_message;
        UPDATE public.tasks
           SET status = 'dlq', dlq_reason = dlq_message, last_error = p_message,
               finished_at = db_now, updated_at = db_now,
               leased_by = NULL, lease_expires_at = NULL
         WHERE id = p_task_id;
        final_status := 'dlq';
        retry_at := NULL;
        attempt := t.attempt;
        worker_id := caller_worker_id;
        RETURN NEXT;
        RETURN;
    END IF;

    delay_seconds := GREATEST(t.retry_delay_seconds, 1)::NUMERIC;
    FOR i IN 2..GREATEST(t.attempt, 1) LOOP
        delay_seconds := trunc(delay_seconds * GREATEST(t.retry_backoff_multiplier, 1)::NUMERIC);
        IF t.retry_max_delay_seconds > 0 AND delay_seconds >= t.retry_max_delay_seconds THEN
            delay_seconds := t.retry_max_delay_seconds;
            EXIT;
        END IF;
        delay_seconds := LEAST(delay_seconds, 31536000);
    END LOOP;
    IF t.retry_max_delay_seconds > 0 THEN
        delay_seconds := LEAST(delay_seconds, t.retry_max_delay_seconds);
    END IF;
    delay_us := (delay_seconds * 1000000)::BIGINT;
    IF t.retry_jitter THEN
        delay_us := floor(random() * (delay_us + 1))::BIGINT;
    END IF;
    retry_at := db_now + (delay_us * interval '1 microsecond');
    UPDATE public.tasks
       SET status = 'pending', last_error = p_message,
           next_run_at = retry_at, updated_at = db_now,
           leased_by = NULL, lease_expires_at = NULL
     WHERE id = p_task_id;
    final_status := 'pending';
    attempt := t.attempt;
    worker_id := caller_worker_id;
    RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_renew(
    p_task_id TEXT, p_lease_version INTEGER, p_lease_timeout_us BIGINT
)
RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT := public.tasklab_worker_identity();
    changed INTEGER;
BEGIN
    IF p_lease_timeout_us <= 0 OR p_lease_timeout_us > 86400000000 THEN
        RAISE EXCEPTION 'invalid lease duration' USING ERRCODE = '22023';
    END IF;
    UPDATE public.tasks
       SET lease_expires_at = clock_timestamp() + (p_lease_timeout_us * interval '1 microsecond'),
           updated_at = clock_timestamp()
     WHERE id = p_task_id AND status = 'running'
       AND leased_by = caller_worker_id AND lease_version = p_lease_version
       AND lease_expires_at >= clock_timestamp();
    GET DIAGNOSTICS changed = ROW_COUNT;
    RETURN changed;
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_insert_event(
    p_task_id TEXT, p_event_type TEXT, p_message TEXT, p_metadata JSONB
)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT := public.tasklab_worker_identity();
    task_status TEXT;
    task_lease_owner TEXT;
    task_lease_version INTEGER;
    task_attempt INTEGER;
    task_last_error TEXT;
BEGIN
    IF p_event_type <> 'priority_promoted' THEN
        RAISE EXCEPTION 'Worker may only append a priority-promotion event' USING ERRCODE = '42501';
    END IF;
    SELECT t.status, t.leased_by, t.lease_version, t.attempt, t.last_error
      INTO task_status, task_lease_owner, task_lease_version, task_attempt, task_last_error
      FROM public.tasks AS t WHERE t.id = p_task_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'task does not exist' USING ERRCODE = '23503';
    END IF;
    IF p_event_type = 'priority_promoted' AND
       (task_status <> 'running' OR task_lease_owner <> caller_worker_id OR
        COALESCE(p_metadata->>'lease_version', '') <> task_lease_version::TEXT OR
        EXISTS (SELECT 1 FROM public.task_events AS e WHERE e.task_id = p_task_id
                  AND e.event_type = 'priority_promoted' AND e.worker_id = caller_worker_id
                  AND e.metadata->>'lease_version' = p_metadata->>'lease_version')) THEN
        RAISE EXCEPTION 'claim event does not match the authenticated live lease' USING ERRCODE = '42501';
    END IF;
    INSERT INTO public.task_events(task_id, event_type, worker_id, message, metadata)
    VALUES (p_task_id, p_event_type, caller_worker_id, p_message, COALESCE(p_metadata, '{}'::JSONB));
END;
$$;

CREATE OR REPLACE FUNCTION tasklab_worker_record_transition()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    caller_worker_id TEXT;
    event_type TEXT;
    event_message TEXT;
    event_metadata JSONB;
BEGIN
    SELECT i.worker_id INTO caller_worker_id
      FROM public.worker_db_identities AS i
     WHERE i.login_role = session_user::NAME;
    IF caller_worker_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF NEW.status = 'running' AND (OLD.status <> NEW.status OR OLD.lease_version <> NEW.lease_version) THEN
        event_type := 'claimed';
        event_message := 'task claimed';
        event_metadata := jsonb_build_object('attempt', NEW.attempt, 'lease_version', NEW.lease_version,
            'claim_reason', CASE WHEN OLD.status = 'running' THEN 'expired_lease' ELSE 'pending' END);
    ELSIF NEW.status = 'success' AND OLD.status = 'running' THEN
        event_type := 'succeeded';
        event_message := 'task completed';
        event_metadata := jsonb_build_object('summary', COALESCE(NEW.result->>'summary', ''), 'lease_version', OLD.lease_version);
    ELSIF NEW.status = 'pending' AND OLD.status = 'running' THEN
        event_type := 'retried';
        event_message := COALESCE(NEW.last_error, '');
        event_metadata := jsonb_build_object('next_retry_at', NEW.next_run_at, 'attempt', NEW.attempt, 'lease_version', OLD.lease_version,
            'error_type', current_setting('tasklab.failure_type', true));
    ELSIF NEW.status = 'dlq' AND OLD.status = 'running' THEN
        event_type := 'moved_to_dlq';
        event_message := COALESCE(NEW.dlq_reason, '');
        event_metadata := jsonb_build_object('error', COALESCE(NEW.last_error, ''), 'lease_version', OLD.lease_version,
            'error_type', current_setting('tasklab.failure_type', true));
    END IF;
    IF event_type IS NOT NULL THEN
        INSERT INTO public.task_events(task_id, event_type, worker_id, message, metadata)
        VALUES (NEW.id, event_type, caller_worker_id, event_message, COALESCE(event_metadata, '{}'::JSONB));
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS tasklab_worker_record_transition ON public.tasks;
CREATE TRIGGER tasklab_worker_record_transition
AFTER UPDATE OF status, lease_version ON public.tasks
FOR EACH ROW EXECUTE FUNCTION tasklab_worker_record_transition();

REVOKE ALL ON FUNCTION tasklab_worker_identity() FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_terminalize_exhausted_leases() FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_claim_task(TEXT, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_ack(TEXT, INTEGER, JSONB) FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_fail(TEXT, INTEGER, TEXT, TEXT, BOOLEAN) FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_renew(TEXT, INTEGER, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_insert_event(TEXT, TEXT, TEXT, JSONB) FROM PUBLIC;
REVOKE ALL ON FUNCTION tasklab_worker_record_transition() FROM PUBLIC;
