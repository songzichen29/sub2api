SET @col_exists = (SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'user_subscriptions' AND column_name = 'redeem_code_id');
SET @ddl = IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `redeem_code_id` BIGINT NULL', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @index_exists = (SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'user_subscriptions' AND index_name = 'user_subscriptions_redeem_code_unique');
SET @ddl = IF(@index_exists = 0,
    'CREATE UNIQUE INDEX `user_subscriptions_redeem_code_unique` ON `user_subscriptions` (`redeem_code_id`)', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
