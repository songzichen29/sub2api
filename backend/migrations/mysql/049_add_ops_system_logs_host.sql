SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_system_logs'
      AND column_name = 'host'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `ops_system_logs` ADD COLUMN `host` VARCHAR(255) NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_system_logs'
      AND index_name = 'idx_ops_system_logs_host_created_at'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_ops_system_logs_host_created_at` ON `ops_system_logs` (`host`, `created_at` DESC)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
