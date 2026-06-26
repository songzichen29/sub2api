ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_weekend_skip BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS skip_weekends BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS weekend_skip_user_changed_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS weekend_skip_original_expires_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS weekend_skip_admin_updated_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS weekend_skip_admin_updated_by BIGINT NULL;

COMMENT ON COLUMN groups.allow_weekend_skip IS 'Allow users to enable weekend skip for subscriptions in this group';
COMMENT ON COLUMN user_subscriptions.skip_weekends IS 'Whether this subscription is unavailable on weekends and expiry is compensated';
COMMENT ON COLUMN user_subscriptions.weekend_skip_user_changed_at IS 'When the user consumed the one-time weekend skip change opportunity';
COMMENT ON COLUMN user_subscriptions.weekend_skip_original_expires_at IS 'Original expires_at before weekend skip first compensated the subscription';
COMMENT ON COLUMN user_subscriptions.weekend_skip_admin_updated_at IS 'When an administrator last changed weekend skip state';
COMMENT ON COLUMN user_subscriptions.weekend_skip_admin_updated_by IS 'Administrator ID that last changed weekend skip state';
