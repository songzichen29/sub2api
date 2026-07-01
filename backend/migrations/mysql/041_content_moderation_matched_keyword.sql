SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'content_moderation_logs'
      AND column_name = 'matched_keyword'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `content_moderation_logs` ADD COLUMN `matched_keyword` VARCHAR(255) NOT NULL DEFAULT '''' AFTER `queue_delay_ms`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
