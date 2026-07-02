SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND column_name = 'parent_account_id'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `accounts` ADD COLUMN `parent_account_id` BIGINT NULL COMMENT ''Parent account id for a linked spark shadow (NULL = normal)'' AFTER `tags`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND column_name = 'quota_dimension'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `accounts` ADD COLUMN `quota_dimension` VARCHAR(20) NOT NULL DEFAULT ''global'' COMMENT ''global (default) or spark shadow quota dimension'' AFTER `parent_account_id`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @constraint_exists := (
    SELECT COUNT(*)
    FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'accounts'
      AND constraint_name = 'chk_accounts_quota_dimension'
      AND constraint_type = 'CHECK'
);

SET @ddl := IF(@constraint_exists = 0,
    'ALTER TABLE `accounts` ADD CONSTRAINT `chk_accounts_quota_dimension` CHECK (`quota_dimension` IN (''global'', ''spark''))',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @constraint_exists := (
    SELECT COUNT(*)
    FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'accounts'
      AND constraint_name = 'chk_accounts_parent_dimension'
      AND constraint_type = 'CHECK'
);

SET @ddl := IF(@constraint_exists = 0,
    'ALTER TABLE `accounts` ADD CONSTRAINT `chk_accounts_parent_dimension` CHECK (((`parent_account_id` IS NULL) AND (`quota_dimension` = ''global'')) OR ((`parent_account_id` IS NOT NULL) AND (`quota_dimension` <> ''global'')))',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- [INFO] MySQL 限制：CHECK 约束不能引用 AUTO_INCREMENT 列（Error 3818），
-- accounts.id 为自增主键，故「父账号不能是自己」不在 DB 层强制；
-- 改由应用层校验 + 外键 fk_accounts_parent_account_id 兜底（PG 侧 154 仍保留该约束）。

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND column_name = 'spark_shadow_parent_key'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `accounts` ADD COLUMN `spark_shadow_parent_key` BIGINT GENERATED ALWAYS AS (CASE WHEN `quota_dimension` = ''spark'' AND `deleted_at` IS NULL THEN `parent_account_id` ELSE NULL END) STORED COMMENT ''Unique active spark shadow parent key''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND index_name = 'idx_accounts_parent_account_id'
);

SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_accounts_parent_account_id` ON `accounts` (`parent_account_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND index_name = 'uq_accounts_spark_shadow_per_parent'
);

SET @ddl := IF(@idx_exists = 0,
    'CREATE UNIQUE INDEX `uq_accounts_spark_shadow_per_parent` ON `accounts` (`spark_shadow_parent_key`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @fk_exists := (
    SELECT COUNT(*)
    FROM information_schema.referential_constraints
    WHERE constraint_schema = DATABASE()
      AND constraint_name = 'fk_accounts_parent_account_id'
);

SET @ddl := IF(@fk_exists = 0,
    'ALTER TABLE `accounts` ADD CONSTRAINT `fk_accounts_parent_account_id` FOREIGN KEY (`parent_account_id`) REFERENCES `accounts` (`id`) ON DELETE RESTRICT',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
