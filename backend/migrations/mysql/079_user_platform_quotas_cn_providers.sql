SET @constraint_clause = (
    SELECT LOWER(check_clause) FROM information_schema.check_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'user_platform_quotas'
      AND constraint_name = 'user_platform_quotas_platform_check'
    LIMIT 1
);
SET @constraint_needs_update = IF(
    @constraint_clause IS NULL
    OR @constraint_clause NOT LIKE '%grok%'
    OR @constraint_clause NOT LIKE '%kimi%'
    OR @constraint_clause NOT LIKE '%zhipu%'
    OR @constraint_clause NOT LIKE '%deepseek%',
    1,
    0
);
SET @sql = IF(@constraint_needs_update = 1 AND @constraint_clause IS NOT NULL,
    'ALTER TABLE `user_platform_quotas` DROP CHECK `user_platform_quotas_platform_check`',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(@constraint_needs_update = 1,
    'ALTER TABLE `user_platform_quotas` ADD CONSTRAINT `user_platform_quotas_platform_check` CHECK (`platform` IN (''anthropic'', ''openai'', ''gemini'', ''antigravity'', ''grok'', ''kimi'', ''zhipu'', ''deepseek''))',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
