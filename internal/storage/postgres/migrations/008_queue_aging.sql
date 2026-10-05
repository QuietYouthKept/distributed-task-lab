-- 008_queue_aging.sql
-- Both defaults retain strict priority. A queue must explicitly opt in.
ALTER TABLE queues ADD COLUMN IF NOT EXISTS aging_after_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE queues ADD COLUMN IF NOT EXISTS aging_priority_boost INTEGER NOT NULL DEFAULT 0;

DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_aging_after_check CHECK (aging_after_seconds >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE queues ADD CONSTRAINT queues_aging_boost_check CHECK (aging_priority_boost >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
