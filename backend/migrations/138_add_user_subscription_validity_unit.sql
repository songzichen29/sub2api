-- Store the original subscription validity unit so daily-overdraft accounting
-- can distinguish day-based cards from week/month cards.
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS validity_unit VARCHAR(10) NOT NULL DEFAULT 'day';

COMMENT ON COLUMN user_subscriptions.validity_unit IS 'Original validity unit for overdraft accounting: day/week/month';
