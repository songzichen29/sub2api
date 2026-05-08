-- 135_add_daily_limit_reset_payment.sql
-- 用户自助付费重置订阅当日额度。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS daily_limit_reset_price DECIMAL(20,2);

COMMENT ON COLUMN groups.daily_limit_reset_price IS '用户自助重置订阅当日额度的支付金额（CNY）；NULL/<=0 表示关闭';

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_payment_orders_subscription_id
    ON payment_orders(subscription_id);
