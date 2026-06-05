-- Allow subscription plans to define a fixed end time.
-- When set, paid subscriptions expire at this timestamp instead of now + validity_days.
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'subscription_plans'
      AND column_name = 'expires_at'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `subscription_plans` ADD COLUMN `expires_at` datetime(6) NULL COMMENT ''Fixed subscription plan end time, purchases expire at this timestamp when set'' AFTER `validity_unit`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND column_name = 'subscription_plan_expires_at'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `subscription_plan_expires_at` datetime(6) NULL COMMENT ''Fixed subscription plan end time frozen at order creation'' AFTER `subscription_validity_unit`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
