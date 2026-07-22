SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'prompt_audit_events' AND column_name = 'full_prompt'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `prompt_audit_events` ADD COLUMN `full_prompt` LONGTEXT NOT NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
