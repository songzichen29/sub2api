ALTER TABLE `channel_monitors` MODIFY COLUMN `provider` ENUM('openai','anthropic','gemini','grok','antigravity','kimi','zhipu','deepseek','minimax') NOT NULL;
ALTER TABLE `channel_monitor_request_templates` MODIFY COLUMN `provider` ENUM('openai','anthropic','gemini','grok','antigravity','kimi','zhipu','deepseek','minimax') NOT NULL;

SET @constraint_exists = (SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND table_name = 'user_platform_quotas' AND constraint_name = 'user_platform_quotas_platform_check' AND constraint_type = 'CHECK');
SET @sql = IF(@constraint_exists > 0, 'ALTER TABLE `user_platform_quotas` DROP CHECK `user_platform_quotas_platform_check`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
ALTER TABLE `user_platform_quotas` ADD CONSTRAINT `user_platform_quotas_platform_check` CHECK (`platform` IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax'));

SET @constraint_exists = (SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND table_name = 'composite_model_routes' AND constraint_name = 'chk_composite_model_routes_target_platform' AND constraint_type = 'CHECK');
SET @sql = IF(@constraint_exists > 0, 'ALTER TABLE `composite_model_routes` DROP CHECK `chk_composite_model_routes_target_platform`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
ALTER TABLE `composite_model_routes` ADD CONSTRAINT `chk_composite_model_routes_target_platform` CHECK (`target_platform` IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax'));
