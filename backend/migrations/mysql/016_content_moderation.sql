-- 016_content_moderation.sql
-- MySQL counterpart for PostgreSQL migration 135_content_moderation.sql.
-- Adds the risk-control default setting and the content moderation audit log table.

INSERT IGNORE INTO `settings` (`key`, `value`, `updated_at`)
VALUES ('risk_control_enabled', 'false', NOW());

CREATE TABLE IF NOT EXISTS `content_moderation_logs` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `request_id` varchar(128) NOT NULL DEFAULT '',
    `user_id` bigint NULL,
    `user_email` varchar(255) NOT NULL DEFAULT '',
    `api_key_id` bigint NULL,
    `api_key_name` varchar(100) NOT NULL DEFAULT '',
    `group_id` bigint NULL,
    `group_name` varchar(255) NOT NULL DEFAULT '',
    `endpoint` varchar(128) NOT NULL DEFAULT '',
    `provider` varchar(64) NOT NULL DEFAULT '',
    `model` varchar(255) NOT NULL DEFAULT '',
    `mode` varchar(32) NOT NULL DEFAULT '',
    `action` varchar(32) NOT NULL DEFAULT '',
    `flagged` bool NOT NULL DEFAULT FALSE,
    `highest_category` varchar(64) NOT NULL DEFAULT '',
    `highest_score` decimal(8,6) NOT NULL DEFAULT 0,
    `category_scores` json NOT NULL,
    `threshold_snapshot` json NOT NULL,
    `input_excerpt` longtext NOT NULL,
    `upstream_latency_ms` int NULL,
    `error` longtext NOT NULL,
    `violation_count` int NOT NULL DEFAULT 0,
    `auto_banned` bool NOT NULL DEFAULT FALSE,
    `email_sent` bool NOT NULL DEFAULT FALSE,
    `queue_delay_ms` int NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_content_moderation_logs_created_at` (`created_at`),
    INDEX `idx_content_moderation_logs_group_created_at` (`group_id`, `created_at`),
    INDEX `idx_content_moderation_logs_flagged_created_at` (`flagged`, `created_at`),
    INDEX `idx_content_moderation_logs_user_created_at` (`user_id`, `created_at`),
    INDEX `idx_content_moderation_logs_api_key_created_at` (`api_key_id`, `created_at`),
    INDEX `idx_content_moderation_logs_endpoint_created_at` (`endpoint`, `created_at`),
    CONSTRAINT `content_moderation_logs_users_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL,
    CONSTRAINT `content_moderation_logs_api_keys_api_key` FOREIGN KEY (`api_key_id`) REFERENCES `api_keys` (`id`) ON DELETE SET NULL,
    CONSTRAINT `content_moderation_logs_groups_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'violation_count'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `violation_count` int NOT NULL DEFAULT 0',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'auto_banned'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `auto_banned` bool NOT NULL DEFAULT FALSE',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'email_sent'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `email_sent` bool NOT NULL DEFAULT FALSE',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'queue_delay_ms'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `queue_delay_ms` int NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
