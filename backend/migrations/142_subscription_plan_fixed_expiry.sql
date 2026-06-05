-- Allow subscription plans to define a fixed end time.
-- When set, paid subscriptions expire at this timestamp instead of now + validity_days.
ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

COMMENT ON COLUMN subscription_plans.expires_at IS 'Fixed subscription plan end time; purchases expire at this timestamp when set';

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_plan_expires_at TIMESTAMPTZ;

COMMENT ON COLUMN payment_orders.subscription_plan_expires_at IS 'Fixed subscription plan end time frozen at order creation';
