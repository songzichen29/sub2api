-- Freeze the subscription plan validity unit at order creation time.
ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_validity_unit VARCHAR(10);

COMMENT ON COLUMN payment_orders.subscription_validity_unit IS 'Original subscription plan validity unit at order creation';
