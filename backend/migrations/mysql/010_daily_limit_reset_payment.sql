-- 010_daily_limit_reset_payment.sql
-- 用户自助付费重置订阅当日额度，对齐 PostgreSQL 端 135_add_daily_limit_reset_payment.sql。

-- groups.daily_limit_reset_price
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'daily_limit_reset_price'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `daily_limit_reset_price` decimal(20,2) NULL COMMENT ''用户自助重置订阅当日额度的支付金额（CNY）；NULL/<=0 表示关闭''',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- payment_orders.subscription_id
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND column_name = 'subscription_id'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `subscription_id` bigint NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- idx_payment_orders_subscription_id
SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND index_name = 'idx_payment_orders_subscription_id'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_payment_orders_subscription_id` ON `payment_orders` (`subscription_id`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
