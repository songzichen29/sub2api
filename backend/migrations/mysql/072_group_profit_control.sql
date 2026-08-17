SET @profit_enabled_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'profit_control_enabled'
);
SET @profit_enabled_sql = IF(@profit_enabled_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `profit_control_enabled` BOOLEAN NOT NULL DEFAULT FALSE', 'SELECT 1');
PREPARE stmt FROM @profit_enabled_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @profit_margin_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'profit_min_margin'
);
SET @profit_margin_sql = IF(@profit_margin_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `profit_min_margin` DECIMAL(10,4) NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE stmt FROM @profit_margin_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @profit_buffer_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'profit_safety_buffer'
);
SET @profit_buffer_sql = IF(@profit_buffer_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `profit_safety_buffer` DECIMAL(10,4) NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE stmt FROM @profit_buffer_sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
