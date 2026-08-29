SET @constraint_exists = (
    SELECT COUNT(1) FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'composite_model_routes'
      AND constraint_name = 'chk_composite_model_routes_target_platform'
      AND constraint_type = 'CHECK'
);
SET @sql = IF(@constraint_exists = 0, 'SELECT 1',
    'ALTER TABLE `composite_model_routes` DROP CHECK `chk_composite_model_routes_target_platform`');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

ALTER TABLE `composite_model_routes`
    ADD CONSTRAINT `chk_composite_model_routes_target_platform`
    CHECK (`target_platform` IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek'));
