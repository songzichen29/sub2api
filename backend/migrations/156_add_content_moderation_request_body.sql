-- Add request session audit bundle storage for flagged content moderation logs.

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS request_body TEXT;

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS request_body_message_count INTEGER NOT NULL DEFAULT 0;
