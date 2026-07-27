SET @request_type_check_exists = (
    SELECT COUNT(1) FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND constraint_name = 'chk_usage_logs_request_type'
      AND constraint_type = 'CHECK'
);
SET @request_type_check_sql = IF(
    @request_type_check_exists = 0,
    'ALTER TABLE `usage_logs` ADD CONSTRAINT `chk_usage_logs_request_type` CHECK (`request_type` >= 0 AND `request_type` <= 5)',
    'SELECT 1'
);
PREPARE stmt FROM @request_type_check_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
