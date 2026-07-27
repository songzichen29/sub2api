SET @allow_live_col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'allow_live'
);
SET @allow_live_sql = IF(
    @allow_live_col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `allow_live` BOOLEAN NOT NULL DEFAULT FALSE AFTER `allow_image_generation`',
    'SELECT 1'
);
PREPARE stmt FROM @allow_live_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
