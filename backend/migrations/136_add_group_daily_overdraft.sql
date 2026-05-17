-- Add daily overdraft switch for subscription groups.
-- When enabled, daily_limit_usd becomes a soft/base daily quota while weekly/monthly
-- limits remain the hard period pool.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_daily_overdraft BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN groups.allow_daily_overdraft IS 'Allow subscription daily quota overdraft into weekly/monthly period pool';
