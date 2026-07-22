SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'duplicate_operation_id'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `duplicate_operation_id` VARCHAR(64) NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'duplicate_operation_active'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `duplicate_operation_active` VARCHAR(64) GENERATED ALWAYS AS (CASE WHEN `deleted_at` IS NULL THEN `duplicate_operation_id` ELSE NULL END) STORED',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND index_name = 'idx_groups_duplicate_operation_id_active'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE UNIQUE INDEX `idx_groups_duplicate_operation_id_active` ON `groups` (`duplicate_operation_active`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
