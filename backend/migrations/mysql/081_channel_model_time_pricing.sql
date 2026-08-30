SET @col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing'
      AND column_name = 'time_pricing'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `channel_model_pricing` ADD COLUMN `time_pricing` JSON NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
