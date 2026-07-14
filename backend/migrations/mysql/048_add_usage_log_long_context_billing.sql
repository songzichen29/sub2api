SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'long_context_billing_applied'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `long_context_billing_applied` BOOLEAN NOT NULL DEFAULT FALSE',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
