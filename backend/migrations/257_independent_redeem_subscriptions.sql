ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS redeem_code_id BIGINT;

CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_redeem_code_unique
    ON user_subscriptions (redeem_code_id);

-- 独立兑换与独立支付均不占用历史合并订阅的唯一位置。
-- 不添加兑换码外键，避免硬删除兑换码时清除来源标记。
DROP INDEX IF EXISTS user_subscriptions_legacy_user_group_unique;
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_legacy_user_group_unique
    ON user_subscriptions (user_id, group_id)
    WHERE deleted_at IS NULL AND purchase_order_id IS NULL AND redeem_code_id IS NULL;
