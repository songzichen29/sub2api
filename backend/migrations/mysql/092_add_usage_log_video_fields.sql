SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'video_count'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `video_count` INT NOT NULL DEFAULT 0',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'video_resolution'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `video_resolution` VARCHAR(10) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'video_duration_seconds'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `video_duration_seconds` INT NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
