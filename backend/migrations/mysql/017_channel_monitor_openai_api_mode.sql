-- 017_channel_monitor_openai_api_mode.sql
-- MySQL counterpart for PostgreSQL migration 138_channel_monitor_openai_api_mode.sql.
-- Adds explicit OpenAI protocol mode to channel monitors and request templates.

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitors'
      AND column_name = 'api_mode'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `channel_monitors` ADD COLUMN `api_mode` varchar(32) NOT NULL DEFAULT ''chat_completions'' AFTER `provider`',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `channel_monitors`
   SET `api_mode` = 'chat_completions'
 WHERE `api_mode` IS NULL
    OR `api_mode` NOT IN ('chat_completions', 'responses');

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitor_request_templates'
      AND column_name = 'api_mode'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `channel_monitor_request_templates` ADD COLUMN `api_mode` varchar(32) NOT NULL DEFAULT ''chat_completions'' AFTER `provider`',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE `channel_monitor_request_templates`
   SET `api_mode` = 'chat_completions'
 WHERE `api_mode` IS NULL
    OR `api_mode` NOT IN ('chat_completions', 'responses');

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitors'
      AND index_name = 'idx_channel_monitors_provider_api_mode'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_channel_monitors_provider_api_mode` ON `channel_monitors` (`provider`, `api_mode`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitor_request_templates'
      AND index_name = 'idx_channel_monitor_templates_provider_api_mode'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_channel_monitor_templates_provider_api_mode` ON `channel_monitor_request_templates` (`provider`, `api_mode`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
