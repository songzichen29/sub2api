SET @provider_enum_type = 'enum(''openai'',''anthropic'',''gemini'',''grok'',''antigravity'',''kimi'',''zhipu'',''deepseek'')';
SET @provider_enum_matches = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitors'
      AND column_name = 'provider'
      AND LOWER(column_type) = @provider_enum_type
);
SET @sql = IF(@provider_enum_matches = 0,
    'ALTER TABLE `channel_monitors` MODIFY COLUMN `provider` ENUM(''openai'',''anthropic'',''gemini'',''grok'',''antigravity'',''kimi'',''zhipu'',''deepseek'') NOT NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @provider_enum_matches = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitor_request_templates'
      AND column_name = 'provider'
      AND LOWER(column_type) = @provider_enum_type
);
SET @sql = IF(@provider_enum_matches = 0,
    'ALTER TABLE `channel_monitor_request_templates` MODIFY COLUMN `provider` ENUM(''openai'',''anthropic'',''gemini'',''grok'',''antigravity'',''kimi'',''zhipu'',''deepseek'') NOT NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'channel_monitors'
      AND column_name = 'check_mode'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `channel_monitors` ADD COLUMN `check_mode` VARCHAR(32) NOT NULL DEFAULT ''probe''',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'channel_monitors'
      AND column_name = 'account_id'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `channel_monitors` ADD COLUMN `account_id` BIGINT NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'channel_monitor_histories'
      AND column_name = 'quota'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `channel_monitor_histories` ADD COLUMN `quota` JSON NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'channel_monitors'
      AND index_name = 'idx_channel_monitors_account_id'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_channel_monitors_account_id` ON `channel_monitors` (`account_id`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @fk_exists = (
    SELECT COUNT(1) FROM information_schema.referential_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'channel_monitors'
      AND constraint_name = 'channel_monitors_account_id_fk'
);
SET @sql = IF(@fk_exists = 0,
    'ALTER TABLE `channel_monitors` ADD CONSTRAINT `channel_monitors_account_id_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE SET NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @check_exists = (
    SELECT COUNT(1) FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE() AND table_name = 'channel_monitors'
      AND constraint_name = 'channel_monitors_check_mode_check'
      AND constraint_type = 'CHECK'
);
SET @sql = IF(@check_exists = 0,
    'ALTER TABLE `channel_monitors` ADD CONSTRAINT `channel_monitors_check_mode_check` CHECK (`check_mode` IN (''probe'', ''quota'', ''quota_probe''))',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

INSERT IGNORE INTO `settings` (`key`, `value`) VALUES ('channel_monitor_show_quota', 'false');
