-- Store the original subscription validity unit so daily-overdraft accounting
-- can distinguish day-based cards from week/month cards.
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_subscriptions'
      AND column_name = 'validity_unit'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `validity_unit` varchar(10) NOT NULL DEFAULT ''day'' COMMENT ''Original validity unit for overdraft accounting: day/week/month'' AFTER `status`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
