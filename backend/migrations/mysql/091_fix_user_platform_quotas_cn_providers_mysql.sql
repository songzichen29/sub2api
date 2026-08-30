SET @constraint_exists = (
    SELECT COUNT(1)
    FROM information_schema.table_constraints
    WHERE table_schema = DATABASE()
      AND constraint_name = 'user_platform_quotas_platform_check'
      AND constraint_type = 'CHECK'
);
SET @sql = IF(@constraint_exists > 0,
    'ALTER TABLE `user_platform_quotas` DROP CHECK `user_platform_quotas_platform_check`',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = 'ALTER TABLE `user_platform_quotas` ADD CONSTRAINT `user_platform_quotas_platform_check` CHECK (`platform` IN (''anthropic'', ''openai'', ''gemini'', ''antigravity'', ''grok'', ''kimi'', ''zhipu'', ''deepseek''))';
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
