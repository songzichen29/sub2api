ALTER TABLE groups ADD COLUMN IF NOT EXISTS user_concurrency_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS fallback_group_id_on_unavailable BIGINT;
