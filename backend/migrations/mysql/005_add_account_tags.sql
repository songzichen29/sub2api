-- 005_add_account_tags.sql
-- 给 accounts 表加管理员维度的标签字段（字符串数组，MySQL 用 JSON 列存储）。
--
-- accounts.tags：自由打的管理员标签集合，仅用于列表筛选和视觉识别。
-- 明确不参与调度 / 权限 / 计费链路 —— 见 feature design
-- easysdd/features/2026-05-04-account-tags/account-tags-design.md。
--
-- 与原 PostgreSQL 版本（根目录 134_add_account_tags.sql）的差异：
--   1. 类型 JSONB → JSON（MySQL 8.x 原生 JSON 类型）。
--   2. 不建 GIN 索引（MySQL 不支持）；筛选走 JSON_CONTAINS（全表扫，
--      account_repo.go ListWithFilters 注释里已说明规模可控）。
--   3. 使用 INFORMATION_SCHEMA + PREPARE/EXECUTE 实现幂等
--      （MySQL 不支持 ALTER TABLE ... ADD COLUMN IF NOT EXISTS）。
--   4. JSON 列 NOT NULL 不能直接带 ADD COLUMN DEFAULT 表达式（5.7
--      / 8.0.13- 限制），分三步：先 NULL → UPDATE 回填 [] → MODIFY NOT NULL。

-- 步骤 1：幂等地添加 tags 列，初始允许 NULL（已存在则跳过）。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND column_name = 'tags'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `accounts` ADD COLUMN `tags` json NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 2：历史数据兜底——把 NULL 回填为空数组，
-- 避免读取层每次都要做 nil 容错（service / repo / dto 也都做了 fallback，这里是双保险）。
UPDATE `accounts`
   SET `tags` = JSON_ARRAY()
 WHERE `tags` IS NULL;

-- 步骤 3：把列改为 NOT NULL，与 ent schema 保持一致。
-- 仅在当前列仍为 NULLABLE 时执行，避免重复运行报错。
SET @is_nullable = (
    SELECT IS_NULLABLE
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'accounts'
      AND column_name = 'tags'
);
SET @sql = IF(@is_nullable = 'YES',
    'ALTER TABLE `accounts` MODIFY COLUMN `tags` json NOT NULL',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
