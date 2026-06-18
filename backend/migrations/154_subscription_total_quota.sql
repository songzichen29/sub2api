-- Add plan-level total quota and freeze it into paid subscription orders.
-- The quota is copied to user_subscriptions and consumed across the whole subscription period.

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS quota_limit_usd DECIMAL(20,8);

COMMENT ON COLUMN subscription_plans.quota_limit_usd IS 'Total USD quota granted by this plan during the subscription period';

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_quota_usd DECIMAL(20,8);

COMMENT ON COLUMN payment_orders.subscription_quota_usd IS 'Subscription quota snapshot frozen at order creation';

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS quota_limit_usd DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS quota_used_usd DECIMAL(20,10) NOT NULL DEFAULT 0;

COMMENT ON COLUMN user_subscriptions.quota_limit_usd IS 'Total USD quota available during this subscription period';
COMMENT ON COLUMN user_subscriptions.quota_used_usd IS 'Total USD quota used during this subscription period';
