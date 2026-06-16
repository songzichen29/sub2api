-- accounts: add scheduler-friendly index for expired accounts that should auto-pause.
-- MySQL has no partial indexes, so include predicate columns in a composite index.

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND index_name = 'idx_accounts_autopause_expiry_due'
);

SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_accounts_autopause_expiry_due` ON `accounts` (`deleted_at`, `schedulable`, `auto_pause_on_expired`, `expires_at`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
