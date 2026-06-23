-- 为订阅套餐添加每人限购次数字段

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'subscription_plans'
      AND column_name = 'max_buy_count'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `subscription_plans` ADD COLUMN `max_buy_count` INT NULL COMMENT ''Per-user purchase limit; NULL means unlimited'' AFTER `features`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
