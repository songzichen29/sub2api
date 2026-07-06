SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'peak_rate_enabled'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `peak_rate_enabled` BOOLEAN NOT NULL DEFAULT FALSE COMMENT ''Whether to enable peak-hour token rate multiplier'' AFTER `rate_multiplier`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'peak_start'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `peak_start` VARCHAR(5) NOT NULL DEFAULT '''' COMMENT ''Peak start time HH:MM'' AFTER `peak_rate_enabled`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'peak_end'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `peak_end` VARCHAR(5) NOT NULL DEFAULT '''' COMMENT ''Peak end time HH:MM, exclusive'' AFTER `peak_start`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'peak_rate_multiplier'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `peak_rate_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 1.0 COMMENT ''Peak-hour token rate multiplier'' AFTER `peak_end`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
