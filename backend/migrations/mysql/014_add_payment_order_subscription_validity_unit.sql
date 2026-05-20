-- Freeze the subscription plan validity unit at order creation time.
-- This avoids classifying week/month subscription orders as day-based overdraft cards
-- if the plan is changed before fulfillment.
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND column_name = 'subscription_validity_unit'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `subscription_validity_unit` varchar(10) NULL COMMENT ''Original subscription plan validity unit at order creation'' AFTER `subscription_days`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
