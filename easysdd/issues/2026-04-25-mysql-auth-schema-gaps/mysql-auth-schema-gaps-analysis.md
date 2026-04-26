---
doc_type: issue-analysis
issue: 2026-04-25-mysql-auth-schema-gaps
status: confirmed
root_cause_type: missing-guard
related: [mysql-auth-schema-gaps-report.md]
tags: [mysql, migrations, auth, schema]
---

# MySQL 迁移遗漏导致鉴权异常 根因分析

## 1. 问题定位

| 关键位置 | 说明 |
|---|---|
| `backend/internal/server/middleware/jwt_auth.go:61` | JWT 中间件在 token 校验后强依赖 `userService.GetByID()` |
| `backend/internal/service/user_service.go:943` | `GetByID()` 在返回用户前会调用 `hydrateUserAvatar()` |
| `backend/internal/service/user_service.go:1023` | `hydrateUserAvatar()` 无条件读取头像表 |
| `backend/internal/repository/user_profile_identity_repo.go:794` | 头像读取直接查询 `user_avatars` |
| `backend/migrations/110_pending_auth_and_provider_default_grants.sql:20` | PostgreSQL 历史 migration 明确创建了 `user_avatars` |
| `backend/migrations/mysql/001_baseline.sql` | 当前 MySQL baseline 未包含 `user_avatars` / `user_provider_default_grants` |

## 2. 失败路径还原

**正常路径**：
登录成功后，JWT 中间件校验 token，读取用户，补齐头像信息，然后把用户上下文写入请求并继续执行后续 handler。

**失败路径**：
登录成功后，JWT 中间件进入 `userService.GetByID()`；`GetByID()` 继续调用 `hydrateUserAvatar()`；repository 查询 `user_avatars` 时因表不存在报错；错误被一路向上返回；中间件把任意用户读取失败都映射成 `USER_NOT_FOUND`，最终用户看到 401。

**分叉点**：`backend/internal/repository/user_profile_identity_repo.go:794` — 查询了 MySQL 当前 schema 中不存在的 `user_avatars` 表。

## 3. 根因

**根因类型**：缺少防御

**根因描述**：
本次 `pgsql -> mysql` 迁移切到 `backend/migrations/mysql/*.sql` 后，运行时实际只应用了 `001_baseline.sql`。该 baseline 由 Ent schema 生成，覆盖了 Ent 管理的表，但遗漏了不在 Ent schema 里的手工 migration 表。`user_avatars` 与 `user_provider_default_grants` 原本由 `110_pending_auth_and_provider_default_grants.sql` 创建，但 MySQL migration 集合中没有对应 follow-up migration，导致数据库 schema 不完整。与此同时，JWT 中间件又把 `GetByID()` 的任意错误都回写成 `USER_NOT_FOUND`，把真正的 schema 问题伪装成了用户不存在。

**是否有多个根因**：是

1. 主根因：MySQL migration 集合遗漏 `110` 对应的手工表
2. 次根因：鉴权路径对头像表缺失没有降级处理
3. 次根因：JWT / Admin JWT 中间件错误映射过粗，掩盖了真实后端错误

## 4. 影响面

- **影响范围**：所有走 `userService.GetByID()` 的 JWT 鉴权接口
- **潜在受害模块**：`/api/v1/auth/me`、公告、usage 等用户侧受保护接口；管理员 JWT 路径也受同类问题影响
- **数据完整性风险**：无直接数据损坏，但 schema 不完整会持续引发运行时错误
- **严重程度复核**：维持 P1

额外对照发现，当前 MySQL baseline 还缺少以下手工 runtime 表：

- `user_group_rate_multipliers`
- `usage_billing_dedup`
- `usage_billing_dedup_archive`
- `channel_pricing_intervals`
- `channel_account_stats_pricing_rules`
- `channel_account_stats_model_pricing`
- `channel_account_stats_pricing_intervals`
- `channel_monitor_aggregation_watermark`
- `auth_identity_migration_reports`
- `user_affiliates`
- `user_affiliate_ledger`

这些表本次没有直接触发当前 401，但后续功能路径可能继续踩中。

## 5. 修复方案

### 方案 A：只改中间件错误码

- **做什么**：把 `USER_NOT_FOUND` 改成 500
- **优点**：改动最小
- **缺点 / 风险**：schema 仍然缺失，鉴权仍会失败，只是错误码更真实
- **影响面**：`jwt_auth.go`、`admin_auth.go`

### 方案 B：补 MySQL follow-up migration + 鉴权路径降级 + 错误映射修正

- **做什么**：新增 MySQL `002` migration 补齐 `user_avatars` / `user_provider_default_grants`；头像表缺失时降级为“无头像”；中间件仅对真正的 `ErrUserNotFound` 返回 401，其余返回 500
- **优点**：既修当前事故，也补根因，并防止未及时迁移时再次把 schema 缺失伪装成用户不存在
- **缺点 / 风险**：改动涉及 migration + middleware + repository
- **影响面**：`backend/migrations/mysql/`、`user_profile_identity_repo.go`、`middleware/*auth*.go`

### 推荐方案

**推荐方案 B**，理由：它同时覆盖了“补 schema”“恢复当前接口”“暴露真实错误”三层问题，且改动仍然收敛在本次事故链路内。
