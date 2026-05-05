-- 给 accounts 表加管理员维度的标签字段（字符串数组，PostgreSQL 用 JSONB 列存储）。
--
-- accounts.tags：管理员自由打的标签集合，仅用于列表筛选与视觉识别。
-- 明确不参与调度 / 权限 / 计费链路 —— 见 feature design
-- easysdd/features/2026-05-04-account-tags/account-tags-design.md。
--
-- 与 MySQL 版本（mysql/005_add_account_tags.sql）的差异：
--   1. 类型 JSON → JSONB（PostgreSQL 原生类型，操作符更丰富）。
--   2. 建 GIN 索引 idx_accounts_tags_gin 让 `tags @> '[...]'::jsonb` 这类
--      筛选走索引（account_repo.ListWithFilters 在 PG 上就是用这个查询）。
--   3. ALTER TABLE ... ADD COLUMN IF NOT EXISTS 直接幂等（PG 9.6+ 原生支持）。

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_accounts_tags_gin ON accounts USING GIN (tags);

COMMENT ON COLUMN accounts.tags IS '管理员维度的标签集合（字符串数组），仅用于列表筛选；不参与调度/权限/计费';
