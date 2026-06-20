-- Add payment coupons and order discount fields.

CREATE TABLE IF NOT EXISTS coupons (
    id BIGINT NOT NULL PRIMARY KEY,
    code VARCHAR(32) NOT NULL UNIQUE,
    type VARCHAR(20) NOT NULL,
    value DECIMAL(20,8) NOT NULL,
    min_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    max_discount DECIMAL(20,2) NOT NULL DEFAULT 0,
    scope VARCHAR(20) NOT NULL DEFAULT 'all',
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    per_user_limit INT NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    starts_at TIMESTAMP NULL,
    expires_at TIMESTAMP NULL,
    notes TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_coupons_status ON coupons(status);
CREATE INDEX IF NOT EXISTS idx_coupons_scope ON coupons(scope);
CREATE INDEX IF NOT EXISTS idx_coupons_expires_at ON coupons(expires_at);

CREATE TABLE IF NOT EXISTS coupon_usages (
    id BIGINT NOT NULL PRIMARY KEY,
    coupon_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL UNIQUE,
    discount_amount DECIMAL(20,2) NOT NULL,
    used_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(20) NOT NULL DEFAULT 'used'
);

CREATE INDEX IF NOT EXISTS idx_coupon_usages_coupon_id ON coupon_usages(coupon_id);
CREATE INDEX IF NOT EXISTS idx_coupon_usages_user_id ON coupon_usages(user_id);
CREATE INDEX IF NOT EXISTS idx_coupon_usages_coupon_user ON coupon_usages(coupon_id, user_id);
CREATE INDEX IF NOT EXISTS idx_coupon_usages_status ON coupon_usages(status);

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS discount_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS coupon_code VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS coupon_discount_amount DECIMAL(20,2) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_payment_orders_coupon_code ON payment_orders(coupon_code);
