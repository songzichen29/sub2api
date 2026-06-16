-- channel_monitors: add jitter_seconds for randomized monitor intervals.

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'channel_monitors'
      AND column_name = 'jitter_seconds'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `channel_monitors` ADD COLUMN `jitter_seconds` int NOT NULL DEFAULT 0 AFTER `interval_seconds`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
