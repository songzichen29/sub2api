SET @email_alias_col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'users' AND column_name = 'email_dot_stripped'
);
SET @email_alias_col_sql = IF(
    @email_alias_col_exists = 0,
    'ALTER TABLE `users` ADD COLUMN `email_dot_stripped` VARCHAR(255) GENERATED ALWAYS AS (REPLACE(LOWER(TRIM(`email`)), ''.'', '''')) STORED',
    'SELECT 1'
);
PREPARE stmt FROM @email_alias_col_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @email_alias_idx_exists = (
    SELECT COUNT(1) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'users' AND index_name = 'idx_users_email_dot_stripped'
);
SET @email_alias_idx_sql = IF(
    @email_alias_idx_exists = 0,
    'CREATE INDEX `idx_users_email_dot_stripped` ON `users` (`email_dot_stripped`, `deleted_at`)',
    'SELECT 1'
);
PREPARE stmt FROM @email_alias_idx_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
