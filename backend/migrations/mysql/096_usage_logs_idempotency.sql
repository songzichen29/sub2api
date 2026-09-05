-- Preserve usage-log idempotency on MySQL without collapsing rows that have no request id.
-- request_id is NOT NULL in the legacy schema, so empty request ids are represented
-- as NULL in a generated key and remain freely insertable.

UPDATE `usage_logs` AS duplicate
JOIN `usage_logs` AS keeper
    ON keeper.`request_id` = duplicate.`request_id`
   AND keeper.`api_key_id` = duplicate.`api_key_id`
   AND keeper.`id` < duplicate.`id`
SET duplicate.`request_id` = ''
WHERE duplicate.`request_id` <> '';

SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'request_id_dedup'
);
SET @sql = IF(
    @col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `request_id_dedup` VARCHAR(64) GENERATED ALWAYS AS (NULLIF(`request_id`, '''')) STORED',
    'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND index_name = 'idx_usage_logs_request_id_api_key_unique'
);
SET @sql = IF(
    @idx_exists = 0,
    'CREATE UNIQUE INDEX `idx_usage_logs_request_id_api_key_unique` ON `usage_logs` (`request_id_dedup`, `api_key_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
