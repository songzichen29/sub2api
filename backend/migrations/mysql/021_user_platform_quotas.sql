-- 用户平台配额表 (MySQL 版本): 按 (user_id, platform) 管理每日/每周/每月 USD 限额
-- NULL limit = 不限额度, 0 = 禁止使用, >0 = USD 限额
-- 使用 deleted_at IS NULL + 生成列 active_platform 模拟 PostgreSQL partial unique index

CREATE TABLE IF NOT EXISTS `user_platform_quotas` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `platform` VARCHAR(32) NOT NULL,

    `daily_limit_usd` DECIMAL(20,10) NULL,
    `weekly_limit_usd` DECIMAL(20,10) NULL,
    `monthly_limit_usd` DECIMAL(20,10) NULL,

    `daily_usage_usd` DECIMAL(20,10) NOT NULL DEFAULT 0,
    `weekly_usage_usd` DECIMAL(20,10) NOT NULL DEFAULT 0,
    `monthly_usage_usd` DECIMAL(20,10) NOT NULL DEFAULT 0,

    `daily_window_start` DATETIME(6) NULL,
    `weekly_window_start` DATETIME(6) NULL,
    `monthly_window_start` DATETIME(6) NULL,

    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `deleted_at` DATETIME(6) NULL,
    `active_platform` VARCHAR(32) GENERATED ALWAYS AS (IF(`deleted_at` IS NULL, `platform`, NULL)) STORED,

    PRIMARY KEY (`id`),
    CONSTRAINT `user_platform_quotas_users_platform_quotas` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
    CONSTRAINT `user_platform_quotas_platform_check` CHECK (`platform` IN ('anthropic', 'openai', 'gemini', 'antigravity')),
    UNIQUE INDEX `userplatformquota_user_id_platform_uq` (`user_id`, `active_platform`),
    INDEX `userplatformquota_user_id` (`user_id`),
    INDEX `userplatformquota_deleted_at` (`deleted_at`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;
