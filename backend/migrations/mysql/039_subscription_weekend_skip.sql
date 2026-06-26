SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'allow_weekend_skip'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `allow_weekend_skip` BOOLEAN NOT NULL DEFAULT FALSE COMMENT ''Allow users to enable weekend skip for subscriptions in this group'' AFTER `allow_daily_overdraft`',
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
      AND column_name = 'skip_weekends'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `skip_weekends` BOOLEAN NOT NULL DEFAULT FALSE COMMENT ''Whether this subscription is unavailable on weekends and expiry is compensated'' AFTER `allow_daily_overdraft`',
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
      AND column_name = 'weekend_skip_user_changed_at'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `weekend_skip_user_changed_at` DATETIME(6) NULL COMMENT ''When the user consumed the one-time weekend skip change opportunity'' AFTER `skip_weekends`',
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
      AND column_name = 'weekend_skip_original_expires_at'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `weekend_skip_original_expires_at` DATETIME(6) NULL COMMENT ''Original expires_at before weekend skip first compensated the subscription'' AFTER `weekend_skip_user_changed_at`',
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
      AND column_name = 'weekend_skip_admin_updated_at'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `weekend_skip_admin_updated_at` DATETIME(6) NULL COMMENT ''When an administrator last changed weekend skip state'' AFTER `weekend_skip_original_expires_at`',
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
      AND column_name = 'weekend_skip_admin_updated_by'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `weekend_skip_admin_updated_by` BIGINT NULL COMMENT ''Administrator ID that last changed weekend skip state'' AFTER `weekend_skip_admin_updated_at`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
