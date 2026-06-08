-- proxies: 有效期 + 失败回退（MySQL 版本）

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'proxies'
      AND column_name = 'expires_at'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `proxies` ADD COLUMN `expires_at` datetime(6) NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'proxies'
      AND column_name = 'fallback_mode'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `proxies` ADD COLUMN `fallback_mode` varchar(20) NOT NULL DEFAULT ''none''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'proxies'
      AND column_name = 'backup_proxy_id'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `proxies` ADD COLUMN `backup_proxy_id` bigint NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'proxies'
      AND column_name = 'expiry_warn_days'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `proxies` ADD COLUMN `expiry_warn_days` int NOT NULL DEFAULT 7',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'proxies'
      AND index_name = 'proxy_expires_at'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `proxy_expires_at` ON `proxies` (`expires_at`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'proxies'
      AND index_name = 'proxy_backup_proxy_id'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `proxy_backup_proxy_id` ON `proxies` (`backup_proxy_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @fk_exists := (
    SELECT COUNT(*)
    FROM information_schema.table_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'proxies'
      AND constraint_name = 'proxies_proxies_backup_proxy'
      AND constraint_type = 'FOREIGN KEY'
);
SET @ddl := IF(@fk_exists = 0,
    'ALTER TABLE `proxies` ADD CONSTRAINT `proxies_proxies_backup_proxy` FOREIGN KEY (`backup_proxy_id`) REFERENCES `proxies` (`id`) ON DELETE SET NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- accounts: fallback 来源（手动回切用）
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND column_name = 'proxy_fallback_origin_id'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `accounts` ADD COLUMN `proxy_fallback_origin_id` bigint NULL',
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
      AND index_name = 'account_proxy_fallback_origin_id'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `account_proxy_fallback_origin_id` ON `accounts` (`proxy_fallback_origin_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
