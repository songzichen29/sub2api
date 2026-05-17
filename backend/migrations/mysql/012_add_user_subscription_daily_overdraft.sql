-- user_subscriptions.allow_daily_overdraft
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_subscriptions'
      AND column_name = 'allow_daily_overdraft'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `allow_daily_overdraft` boolean NOT NULL DEFAULT FALSE COMMENT ''Whether this user subscription has enabled daily quota overdraft''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
