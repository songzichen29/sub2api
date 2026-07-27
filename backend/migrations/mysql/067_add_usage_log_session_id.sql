SET @usage_session_col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'session_id'
);
SET @usage_session_sql = IF(
    @usage_session_col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `session_id` VARCHAR(255) NULL AFTER `ip_address`',
    'SELECT 1'
);
PREPARE stmt FROM @usage_session_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @batch_session_col_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'batch_image_jobs' AND column_name = 'session_id'
);
SET @batch_session_sql = IF(
    @batch_session_col_exists = 0,
    'ALTER TABLE `batch_image_jobs` ADD COLUMN `session_id` VARCHAR(255) NULL AFTER `request_hash`',
    'SELECT 1'
);
PREPARE stmt FROM @batch_session_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
