SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'request_body'
);
SET @ddl = IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `request_body` LONGTEXT NULL COMMENT ''Sanitized request session audit bundle'' AFTER `input_excerpt`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @count_col_exists = (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'request_body_message_count'
);
SET @count_ddl = IF(@count_col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `request_body_message_count` INT NOT NULL DEFAULT 0 COMMENT ''Saved request session message count'' AFTER `request_body`',
    'SELECT 1'
);
PREPARE stmt FROM @count_ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
