-- usage_logs: record the first upstream SSE event arrival time.
--
-- This is intentionally separate from first_token_ms:
-- - upstream_first_event_ms: response.created / first upstream event arrival
-- - first_token_ms: first real non-empty output delta/token

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'upstream_first_event_ms'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `upstream_first_event_ms` int NULL AFTER `first_token_ms`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
