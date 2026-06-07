-- 已删除 API key 审计表:删除 key 时同步留存(明文 key、所有者、key 信息),
-- 供认证失败(INVALID_API_KEY)反查"这个失效 key 曾属于谁"。
-- 仅对本表上线后删除的 key 生效;此前已删的 key 原值已被 tombstone 覆盖,无法补录。

CREATE TABLE IF NOT EXISTS `deleted_api_key_audits` (
    `id`           bigint NOT NULL AUTO_INCREMENT,
    `key`          varchar(128) NOT NULL,
    `api_key_id`   bigint NOT NULL,
    `user_id`      bigint NOT NULL,
    `key_name`     varchar(100) NOT NULL DEFAULT '',
    `deleted_at`   datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `created_at`   datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_deleted_api_key_audits_key` (`key`),
    INDEX `idx_deleted_api_key_audits_user_id` (`user_id`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ops_error_logs: 添加已删除 key 反查列

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_error_logs'
      AND column_name = 'attempted_key_prefix'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `ops_error_logs` ADD COLUMN `attempted_key_prefix` varchar(32) NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_error_logs'
      AND column_name = 'deleted_key_owner_user_id'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `ops_error_logs` ADD COLUMN `deleted_key_owner_user_id` bigint NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_error_logs'
      AND column_name = 'deleted_key_name'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `ops_error_logs` ADD COLUMN `deleted_key_name` varchar(100) NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
