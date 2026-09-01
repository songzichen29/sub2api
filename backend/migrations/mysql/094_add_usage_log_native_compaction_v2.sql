SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'native_compaction_v2'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `native_compaction_v2` BOOLEAN NOT NULL DEFAULT FALSE',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
