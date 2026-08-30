SET @video_model_prices_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'video_model_prices'
);
SET @sql = IF(@video_model_prices_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `video_model_prices` JSON NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
