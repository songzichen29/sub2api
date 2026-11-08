SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'user_concurrency_limit');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `groups` ADD COLUMN `user_concurrency_limit` INT NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'fallback_group_id_on_unavailable');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `groups` ADD COLUMN `fallback_group_id_on_unavailable` BIGINT NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
