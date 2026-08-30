SET @col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
      AND column_name = 'effective_requested_model'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `effective_requested_model` VARCHAR(100) GENERATED ALWAYS AS (COALESCE(NULLIF(TRIM(`requested_model`), ''''), `model`)) STORED',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
      AND column_name = 'effective_upstream_model'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `effective_upstream_model` VARCHAR(100) GENERATED ALWAYS AS (COALESCE(NULLIF(TRIM(`upstream_model`), ''''), `model`)) STORED',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
      AND index_name = 'idx_usage_logs_effective_requested_model_created'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_usage_logs_effective_requested_model_created` ON `usage_logs` (`effective_requested_model`, `created_at` DESC, `id` DESC)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
      AND index_name = 'idx_usage_logs_effective_upstream_model_created'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_usage_logs_effective_upstream_model_created` ON `usage_logs` (`effective_upstream_model`, `created_at` DESC, `id` DESC)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
