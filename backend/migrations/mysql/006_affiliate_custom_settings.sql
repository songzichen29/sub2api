-- 006_affiliate_custom_settings.sql
-- 给 user_affiliates 表加管理员维度的"专属设置"两列，对齐 PostgreSQL 端的 132_affiliate_custom_settings.sql。
--
-- aff_rebate_rate_percent：用户作为邀请人时的专属返利比例（NULL 表示沿用全局比例）。
-- aff_code_custom：邀请码是否被管理员手动改写过（用于"专属用户"列表筛选）。
--
-- 与原 PostgreSQL 版本的差异：
--   1. 类型映射：DECIMAL(5,2) 与 BOOLEAN 在 MySQL 8.x 与 PG 等价，无需调整。
--   2. 不建 partial index。PG 端用 WHERE 子句的部分索引加速 admin "专属用户" 列表筛选，
--      MySQL 没有 partial index 概念。该列表为管理员低频查询、规模可控
--      （专属用户数远小于全部用户），全表扫不影响——参考 005 同款决策。
--   3. 使用 INFORMATION_SCHEMA + PREPARE/EXECUTE 实现幂等
--      （MySQL 不支持 ALTER TABLE ... ADD COLUMN IF NOT EXISTS）。

-- 步骤 1：幂等地添加 aff_rebate_rate_percent 列。NULL 表示沿用全局比例。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliates'
      AND column_name = 'aff_rebate_rate_percent'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliates` ADD COLUMN `aff_rebate_rate_percent` decimal(5,2) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 2：幂等地添加 aff_code_custom 列。默认 FALSE——历史数据视为系统生成的邀请码。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliates'
      AND column_name = 'aff_code_custom'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliates` ADD COLUMN `aff_code_custom` boolean NOT NULL DEFAULT FALSE',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
