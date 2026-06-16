-- scheduler_outbox: add dedup_key used to suppress duplicate pending snapshot events.

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'scheduler_outbox'
      AND column_name = 'dedup_key'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `scheduler_outbox` ADD COLUMN `dedup_key` varchar(191) NULL AFTER `payload`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
