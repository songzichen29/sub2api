-- 邀请返利：用户专属充值/订阅返利比例
-- 1) aff_recharge_rebate_rate_percent: 用户作为邀请人时，对余额充值生效的专属返利比例
-- 2) aff_subscription_rebate_rate_percent: 用户作为邀请人时，对订阅购买生效的专属返利比例

ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS aff_recharge_rebate_rate_percent DECIMAL(5,2);

ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS aff_subscription_rebate_rate_percent DECIMAL(5,2);

DROP INDEX IF EXISTS idx_user_affiliates_admin_settings;

CREATE INDEX IF NOT EXISTS idx_user_affiliates_admin_settings
    ON user_affiliates (updated_at)
    WHERE aff_code_custom = true
       OR aff_rebate_rate_percent IS NOT NULL
       OR aff_recharge_rebate_rate_percent IS NOT NULL
       OR aff_subscription_rebate_rate_percent IS NOT NULL;

COMMENT ON COLUMN user_affiliates.aff_recharge_rebate_rate_percent IS '专属充值返利比例（百分比 0-100，NULL 表示沿用通用/全局）';
COMMENT ON COLUMN user_affiliates.aff_subscription_rebate_rate_percent IS '专属订阅返利比例（百分比 0-100，NULL 表示沿用通用/全局）';
