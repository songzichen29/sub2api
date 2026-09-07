SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'search_price_per_1k');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `search_price_per_1k` DECIMAL(20,8) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'audio_realtime_price_per_min');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `audio_realtime_price_per_min` DECIMAL(20,8) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'audio_tts_price_per_million_chars');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `audio_tts_price_per_million_chars` DECIMAL(20,8) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'audio_stt_price_per_hour');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `audio_stt_price_per_hour` DECIMAL(20,8) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'long_context_pricing_enabled');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `long_context_pricing_enabled` BOOL NOT NULL DEFAULT true', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'model_pricing');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `model_pricing` JSON NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `groups` SET `long_context_pricing_enabled` = true WHERE `long_context_pricing_enabled` IS NULL;
