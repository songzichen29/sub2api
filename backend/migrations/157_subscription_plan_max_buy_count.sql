-- 为订阅套餐添加每人限购次数字段

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS max_buy_count INTEGER;

COMMENT ON COLUMN subscription_plans.max_buy_count IS 'Per-user purchase limit; NULL means unlimited';
