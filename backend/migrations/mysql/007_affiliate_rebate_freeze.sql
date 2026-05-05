-- 007_affiliate_rebate_freeze.sql
-- 邀请返利冻结期支持，对齐 PostgreSQL 端的 133_affiliate_rebate_freeze.sql。
--
-- 1) user_affiliates.aff_frozen_quota：当前处于冻结期、尚未解冻的返利额度（DECIMAL(20,8)）。
-- 2) user_affiliate_ledger.frozen_until：单条流水的冻结到期时间，NULL 表示无冻结或已解冻。
--
-- 与原 PostgreSQL 版本的差异：
--   1. 类型映射：DECIMAL(20,8) 保持一致；TIMESTAMPTZ → datetime(6)，与 003 中
--      created_at/updated_at 的写法保持统一（MySQL 8 下用 CURRENT_TIMESTAMP(6) 精度）。
--   2. PG 端的 partial index `WHERE frozen_until IS NOT NULL` 在 MySQL 没有等价语法。
--      解冻扫描走 `WHERE frozen_until IS NOT NULL AND frozen_until <= NOW()`
--      （affiliate_repo.go:47/198-201），在 frozen_until 上建普通 BTree 索引即可——
--      NULL 不会进入 BTree 索引，效果接近 partial index。
--   3. 使用 INFORMATION_SCHEMA + PREPARE/EXECUTE 实现幂等
--      （MySQL 不支持 ALTER TABLE ... ADD COLUMN IF NOT EXISTS / CREATE INDEX IF NOT EXISTS）。

-- 步骤 1：给 user_affiliates 加 aff_frozen_quota，默认 0，与 aff_quota / aff_history_quota 同型。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliates'
      AND column_name = 'aff_frozen_quota'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliates` ADD COLUMN `aff_frozen_quota` decimal(20,8) NOT NULL DEFAULT 0',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 2：给 user_affiliate_ledger 加 frozen_until，NULL 表示无冻结或已解冻。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND column_name = 'frozen_until'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `frozen_until` datetime(6) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 3：解冻扫描索引。PG 端是 (user_id, frozen_until) WHERE frozen_until IS NOT NULL；
-- MySQL 用普通索引 (user_id, frozen_until)，NULL 不进 BTree，效果接近。
SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND index_name = 'idx_ual_frozen_thaw'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_ual_frozen_thaw` ON `user_affiliate_ledger` (`user_id`, `frozen_until`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
