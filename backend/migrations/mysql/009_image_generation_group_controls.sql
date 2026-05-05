-- 009_image_generation_group_controls.sql
-- 给 groups 表加生图能力 + 图片倍率模式控制，对齐 PostgreSQL 端的
-- 134_image_generation_group_controls.sql（与同号的 134_add_account_tags
-- / 134_affiliate_ledger_audit_snapshots 是 PG 端编号冲突的三个不同迁移
-- ——MySQL 端按已用最大编号 +1 顺延，避免重号）。
--
-- 新增列：
--   allow_image_generation BOOLEAN NOT NULL DEFAULT false
--     是否允许该分组使用图片生成能力（用户 Group.List / 调度器都会读这一列）。
--   image_rate_independent BOOLEAN NOT NULL DEFAULT false
--     图片生成是否使用独立倍率；false 表示共享分组有效倍率。
--   image_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0
--     图片生成独立倍率，仅 image_rate_independent=true 时生效。
--
-- 回填策略（与 PG 134 保持一致）：
--   1. 现有 openai/gemini/antigravity 平台的分组默认开启 allow_image_generation，
--      避免升级后中断已有图片业务。
--   2. image_rate_independent / image_rate_multiplier 的"全表重置"在 PG 端是
--      冗余幂等动作（新列 DEFAULT 已经是 false/1.0），MySQL 端同样保留以与 PG 行为对齐。
--
-- 与原 PostgreSQL 版本的差异：
--   1. 类型映射：BOOLEAN、DECIMAL(10,4) 与 PG 等价。
--   2. PG 的 ADD COLUMN IF NOT EXISTS / COMMENT ON COLUMN 在 MySQL 不可用，
--      用 INFORMATION_SCHEMA + PREPARE/EXECUTE 实现幂等；列注释直接写在 ALTER 里。

-- 步骤 1：allow_image_generation。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'allow_image_generation'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `allow_image_generation` boolean NOT NULL DEFAULT FALSE COMMENT ''是否允许该分组使用图片生成能力''',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 2：image_rate_independent。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'image_rate_independent'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `image_rate_independent` boolean NOT NULL DEFAULT FALSE COMMENT ''图片生成是否使用独立倍率；false 表示共享分组有效倍率''',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 3：image_rate_multiplier。
SET @col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'image_rate_multiplier'
);
SET @sql = IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `image_rate_multiplier` decimal(10,4) NOT NULL DEFAULT 1.0 COMMENT ''图片生成独立倍率，仅 image_rate_independent=true 时生效''',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 步骤 4：openai/gemini/antigravity 平台的现有分组默认开启 allow_image_generation。
-- 注意：本 UPDATE 是回填型操作，**不**是幂等冲突——它只把符合 platform 条件的行改成 true，
-- 不会回滚管理员后续手动关闭的分组（因为该列值已是 true，UPDATE 是 no-op）。
UPDATE `groups`
   SET `allow_image_generation` = TRUE
 WHERE `platform` IN ('openai', 'gemini', 'antigravity')
   AND `allow_image_generation` = FALSE;
