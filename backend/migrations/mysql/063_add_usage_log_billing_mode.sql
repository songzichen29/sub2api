SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'billing_mode'
);
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `usage_logs` ADD COLUMN `billing_mode` varchar(20) NULL AFTER `billing_tier`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
