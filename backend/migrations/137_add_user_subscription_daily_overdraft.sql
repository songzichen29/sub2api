-- Add per-subscription daily overdraft switch.
-- Group allow_daily_overdraft only means the plan supports this capability;
-- this column stores whether the user enabled it for their own subscription.
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS allow_daily_overdraft BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN user_subscriptions.allow_daily_overdraft IS 'Whether this user subscription has enabled daily quota overdraft';
