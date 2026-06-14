-- 默认启用 OpenAI 高级调度器，让 usage_logs.first_token_ms 反馈进入账号选择。
-- 该设置仍可在后台显式改为 false；这里只对缺失 key 的旧库补默认值。

INSERT INTO `settings` (`key`, `value`, `updated_at`)
VALUES ('openai_advanced_scheduler_enabled', 'true', NOW(6))
ON DUPLICATE KEY UPDATE `value` = `value`;
