-- groups.allow_daily_overdraft
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'allow_daily_overdraft'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `allow_daily_overdraft` boolean NOT NULL DEFAULT FALSE COMMENT ''Allow subscription daily quota overdraft into weekly/monthly period pool''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
