SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'force_openai_fast');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `force_openai_fast` BOOLEAN NOT NULL DEFAULT FALSE', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'free_openai_fast');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `free_openai_fast` BOOLEAN NOT NULL DEFAULT FALSE', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'max_reasoning_effort_over_limit');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `max_reasoning_effort_over_limit` VARCHAR(20) NOT NULL DEFAULT ''downgrade''', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'codex_models_manifest_config');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `groups` ADD COLUMN `codex_models_manifest_config` JSON NOT NULL DEFAULT (JSON_OBJECT())', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'upstream_request_id');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `usage_logs` ADD COLUMN `upstream_request_id` VARCHAR(128) NULL', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing' AND column_name = 'cache_write_1h_price');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_model_pricing` ADD COLUMN `cache_write_1h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_pricing_intervals' AND column_name = 'cache_write_1h_price');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_pricing_intervals` ADD COLUMN `cache_write_1h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_account_stats_model_pricing' AND column_name = 'cache_write_1h_price');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_account_stats_model_pricing` ADD COLUMN `cache_write_1h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_account_stats_pricing_intervals' AND column_name = 'cache_write_1h_price');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_account_stats_pricing_intervals` ADD COLUMN `cache_write_1h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing' AND column_name = 'max_reasoning_effort_multiplier');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_model_pricing` ADD COLUMN `max_reasoning_effort_multiplier` DECIMAL(10,4) NULL', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @constraint_exists = (SELECT COUNT(1) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'channel_model_pricing' AND constraint_name = 'chk_channel_model_pricing_max_reasoning_effort_multiplier_positive');
SET @sql = IF(@constraint_exists = 0, 'ALTER TABLE `channel_model_pricing` ADD CONSTRAINT `chk_channel_model_pricing_max_reasoning_effort_multiplier_positive` CHECK (`max_reasoning_effort_multiplier` IS NULL OR `max_reasoning_effort_multiplier` > 0)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @idx_exists = (SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND index_name = 'idx_usage_logs_upstream_request_id');
SET @sql = IF(@idx_exists = 0, 'CREATE INDEX `idx_usage_logs_upstream_request_id` ON `usage_logs` (`upstream_request_id`)', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
