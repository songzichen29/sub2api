ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS account_name VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_content_moderation_logs_request_api_key
    ON content_moderation_logs(request_id, api_key_id);

CREATE INDEX IF NOT EXISTS idx_content_moderation_logs_account_created_at
    ON content_moderation_logs(account_id, created_at DESC);
