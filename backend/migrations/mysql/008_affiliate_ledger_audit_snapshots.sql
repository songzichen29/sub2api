-- 008_affiliate_ledger_audit_snapshots.sql
-- 邀请返利流水审计快照，对齐 PostgreSQL 端的 134_affiliate_ledger_audit_snapshots.sql。
--
-- 给 user_affiliate_ledger 加 5 列：
--   source_order_id           BIGINT NULL  → 产生该返利流水的充值订单 id
--   balance_after             DECIMAL(20,8) NULL → 转余额后的用户余额快照
--   aff_quota_after           DECIMAL(20,8) NULL → 转余额后的可用返利额度快照
--   aff_frozen_quota_after    DECIMAL(20,8) NULL → 转余额后的冻结返利额度快照
--   aff_history_quota_after   DECIMAL(20,8) NULL → 转余额后的历史返利总额快照
-- 五列都允许 NULL，因为历史旧流水无法可靠反推这些值（与 PG 注释对齐）。
--
-- 与原 PostgreSQL 版本的差异：
--   1. 类型映射：BIGINT、DECIMAL(20,8) 完全一致。
--   2. partial index 退化：
--        - PG: idx_user_affiliate_ledger_source_order_id WHERE source_order_id IS NOT NULL
--          → MySQL 改建普通索引 (source_order_id)；NULL 不进 BTree，效果接近。
--        - PG: idx_user_affiliate_ledger_rebate_lookup (...) WHERE action = 'accrue'
--          → MySQL 改建普通复合索引 (action, source_order_id, user_id, source_user_id, created_at)；
--          因为 action 列是首字段，等值查询 'accrue' 会自然走索引，不再需要 partial index。
--   3. **不在迁移中做回填**：PG 端 134 末尾用 LATERAL + EXTRACT(EPOCH FROM)
--      + INTERVAL + 正则 substring 做"尽力回填"，这些语法在 MySQL 全部不支持
--      或语义不等价。强行用 REGEXP_SUBSTR/JSON_EXTRACT 重写有失败风险，
--      一旦失败整个迁移事务回滚、卡住后续迁移链。
--      工程决策：迁移只做不可逆的加列/加索引，回填以独立脚本提供（按需手动执行）。
--   4. 使用 INFORMATION_SCHEMA + PREPARE/EXECUTE 实现幂等。

-- 步骤 1：source_order_id（外键 → payment_orders.id ON DELETE SET NULL）。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND column_name = 'source_order_id'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `source_order_id` bigint NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 外键约束：只在尚未存在时加。
SET @fk_exists = (
    SELECT COUNT(1)
    FROM information_schema.referential_constraints
    WHERE constraint_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND constraint_name = 'user_affiliate_ledger_source_order_fk'
);
SET @sql = IF(@fk_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD CONSTRAINT `user_affiliate_ledger_source_order_fk` FOREIGN KEY (`source_order_id`) REFERENCES `payment_orders` (`id`) ON DELETE SET NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 2：balance_after。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND column_name = 'balance_after'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `balance_after` decimal(20,8) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 3：aff_quota_after。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND column_name = 'aff_quota_after'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `aff_quota_after` decimal(20,8) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 4：aff_frozen_quota_after。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND column_name = 'aff_frozen_quota_after'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `aff_frozen_quota_after` decimal(20,8) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 5：aff_history_quota_after。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND column_name = 'aff_history_quota_after'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `aff_history_quota_after` decimal(20,8) NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 6：source_order_id 单列索引（替代 PG 的 partial index）。
SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND index_name = 'idx_user_affiliate_ledger_source_order_id'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_user_affiliate_ledger_source_order_id` ON `user_affiliate_ledger` (`source_order_id`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 7：返利查询复合索引（替代 PG 的 partial index WHERE action='accrue'）。
SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'user_affiliate_ledger'
      AND index_name = 'idx_user_affiliate_ledger_rebate_lookup'
);
SET @sql = IF(@idx_exists = 0,
    'CREATE INDEX `idx_user_affiliate_ledger_rebate_lookup` ON `user_affiliate_ledger` (`action`, `source_order_id`, `user_id`, `source_user_id`, `created_at`)',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
