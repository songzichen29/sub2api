SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'account_id'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `account_id` BIGINT NULL AFTER `api_key_name`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'account_name'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `account_name` VARCHAR(255) NOT NULL DEFAULT '''' AFTER `account_id`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND index_name = 'idx_content_moderation_logs_request_api_key'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_content_moderation_logs_request_api_key` ON `content_moderation_logs` (`request_id`, `api_key_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND index_name = 'idx_content_moderation_logs_account_created_at'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_content_moderation_logs_account_created_at` ON `content_moderation_logs` (`account_id`, `created_at`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @fk_exists := (
    SELECT COUNT(*)
    FROM information_schema.referential_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND constraint_name = 'content_moderation_logs_accounts_account'
);
SET @ddl := IF(@fk_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD CONSTRAINT `content_moderation_logs_accounts_account` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE SET NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
