SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'image_input_tokens'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `image_input_tokens` INT NOT NULL DEFAULT 0',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'image_input_cost'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `image_input_cost` DECIMAL(20,10) NOT NULL DEFAULT 0',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
