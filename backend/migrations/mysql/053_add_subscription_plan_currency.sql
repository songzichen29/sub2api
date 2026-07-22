SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'subscription_plans' AND column_name = 'currency'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `subscription_plans` ADD COLUMN `currency` VARCHAR(3) NOT NULL DEFAULT ''''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
