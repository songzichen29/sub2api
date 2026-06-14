-- 账号分组调度复合索引（MySQL 版本）
--
-- 上游 PostgreSQL 迁移 150_account_group_scheduler_indexes_notx.sql 使用 PG 专用建索引语法。
-- MySQL 这里通过 information_schema.statistics 做幂等保护。

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'account_groups'
      AND index_name = 'idx_account_groups_group_priority_account'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_account_groups_group_priority_account` ON `account_groups` (`group_id`, `priority`, `account_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'account_groups'
      AND index_name = 'idx_account_groups_account_priority_group'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_account_groups_account_priority_group` ON `account_groups` (`account_id`, `priority`, `group_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
