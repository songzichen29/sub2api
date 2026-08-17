SET @response_model_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'upstream_response_model'
);
SET @response_model_sql = IF(@response_model_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `upstream_response_model` VARCHAR(200) NULL', 'SELECT 1');
PREPARE stmt FROM @response_model_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @model_mismatch_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'upstream_model_mismatch'
);
SET @model_mismatch_sql = IF(@model_mismatch_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `upstream_model_mismatch` BOOLEAN NULL', 'SELECT 1');
PREPARE stmt FROM @model_mismatch_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
