-- scheduler_outbox: unique pending dedup key.
-- MySQL permits multiple NULL values in a unique index, matching the desired "only non-NULL keys dedupe" behavior.

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'scheduler_outbox'
      AND index_name = 'idx_scheduler_outbox_pending_dedup_key'
);

SET @ddl := IF(@idx_exists = 0,
    'CREATE UNIQUE INDEX `idx_scheduler_outbox_pending_dedup_key` ON `scheduler_outbox` (`dedup_key`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
