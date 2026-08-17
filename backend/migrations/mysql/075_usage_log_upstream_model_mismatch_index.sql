SET @model_mismatch_idx_exists = (
    SELECT COUNT(1) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'usage_logs'
      AND index_name = 'idx_usage_logs_upstream_model_mismatch_created_at'
);
SET @model_mismatch_idx_sql = IF(@model_mismatch_idx_exists = 0,
    'CREATE INDEX `idx_usage_logs_upstream_model_mismatch_created_at` ON `usage_logs` (`upstream_model_mismatch`, `created_at` DESC, `id` DESC)',
    'SELECT 1');
PREPARE stmt FROM @model_mismatch_idx_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
