SET @constraint_clause = (
    SELECT LOWER(cc.check_clause)
    FROM information_schema.check_constraints AS cc
    INNER JOIN information_schema.table_constraints AS tc
        ON tc.constraint_schema = cc.constraint_schema
       AND tc.constraint_name = cc.constraint_name
    WHERE cc.constraint_schema = DATABASE()
      AND tc.table_name = 'composite_model_routes'
      AND cc.constraint_name = 'chk_composite_model_routes_target_platform'
      AND tc.constraint_type = 'CHECK'
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
    'ALTER TABLE `composite_model_routes` DROP CHECK `chk_composite_model_routes_target_platform`',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(@constraint_needs_update = 1,
    'ALTER TABLE `composite_model_routes` ADD CONSTRAINT `chk_composite_model_routes_target_platform` CHECK (`target_platform` IN (''anthropic'', ''openai'', ''gemini'', ''antigravity'', ''grok'', ''kimi'', ''zhipu'', ''deepseek''))',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
