-- Add optional absolute expiry timestamp for redeem codes.
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'redeem_codes'
      AND column_name = 'expires_at'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `redeem_codes` ADD COLUMN `expires_at` datetime(6) NULL AFTER `validity_days`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
