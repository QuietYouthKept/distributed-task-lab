-- 006_queue_rate_limit.sql
-- P3 follow-up: token-bucket burst size for per-queue rate limiting.

ALTER TABLE queues ADD COLUMN IF NOT EXISTS burst INTEGER NOT NULL DEFAULT 1;
