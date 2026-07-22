SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'max_reasoning_effort'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `max_reasoning_effort` VARCHAR(20) NOT NULL DEFAULT ''''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'reasoning_effort_mappings'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `reasoning_effort_mappings` JSON NOT NULL DEFAULT (JSON_ARRAY())',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
