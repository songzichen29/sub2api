SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND index_name = 'idx_usage_logs_api_key_latest_ip'
);

SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_usage_logs_api_key_latest_ip` ON `usage_logs` (`api_key_id`, `created_at` DESC, `id` DESC, `ip_address`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
