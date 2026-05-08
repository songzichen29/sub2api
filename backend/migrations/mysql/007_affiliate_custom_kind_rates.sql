-- 007_affiliate_custom_kind_rates.sql
-- 为 user_affiliates 表增加用户级“专属充值返利比例 / 专属订阅返利比例”。

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliates'
      AND column_name = 'aff_recharge_rebate_rate_percent'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliates` ADD COLUMN `aff_recharge_rebate_rate_percent` decimal(5,2) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliates'
      AND column_name = 'aff_subscription_rebate_rate_percent'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliates` ADD COLUMN `aff_subscription_rebate_rate_percent` decimal(5,2) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
