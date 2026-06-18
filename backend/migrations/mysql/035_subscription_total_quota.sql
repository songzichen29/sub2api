-- Add plan-level total quota and freeze it into paid subscription orders.
-- The quota is copied to user_subscriptions and consumed across the whole subscription period.

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'subscription_plans'
      AND column_name = 'quota_limit_usd'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `subscription_plans` ADD COLUMN `quota_limit_usd` DECIMAL(20,8) NULL COMMENT ''Total USD quota granted by this plan during the subscription period'' AFTER `validity_unit`',
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
      AND column_name = 'subscription_quota_usd'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `subscription_quota_usd` DECIMAL(20,8) NULL COMMENT ''Subscription quota snapshot frozen at order creation'' AFTER `subscription_days`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_subscriptions'
      AND column_name = 'quota_limit_usd'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `quota_limit_usd` DECIMAL(20,8) NULL COMMENT ''Total USD quota available during this subscription period'' AFTER `monthly_usage_usd`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_subscriptions'
      AND column_name = 'quota_used_usd'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `quota_used_usd` DECIMAL(20,10) NOT NULL DEFAULT 0 COMMENT ''Total USD quota used during this subscription period'' AFTER `quota_limit_usd`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
