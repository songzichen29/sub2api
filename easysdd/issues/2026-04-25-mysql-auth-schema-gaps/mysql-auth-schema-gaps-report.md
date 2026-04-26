---
doc_type: issue-report
issue: 2026-04-25-mysql-auth-schema-gaps
status: confirmed
severity: P1
summary: MySQL 迁移基线遗漏手工 auth/profile 表，导致登录后鉴权接口误报 USER_NOT_FOUND
tags: [mysql, migrations, auth, schema]
---

# MySQL 迁移遗漏导致鉴权异常 Issue Report

## 1. 问题现象

在 MySQL 环境中，`/api/v1/auth/login` 和 `/api/v1/auth/refresh` 可以返回成功，但带 Bearer Token 访问 `/api/v1/auth/me`、`/api/v1/announcements`、`/api/v1/usage` 等需要登录态的接口时返回 `401`，错误码统一为 `USER_NOT_FOUND`。

## 2. 复现步骤

1. 使用有效账号调用 `/api/v1/auth/login`
2. 拿到 `access_token`
3. 带 `Authorization: Bearer <token>` 访问 `/api/v1/auth/me`
4. 观察到：接口返回 `401 USER_NOT_FOUND`

复现频率：稳定复现

## 3. 期望 vs 实际

**期望行为**：登录成功后，带有效 access token 访问鉴权接口应返回当前用户信息或业务数据。

**实际行为**：登录成功、refresh 成功，但后续鉴权接口统一返回 `401 USER_NOT_FOUND`。

## 4. 环境信息

- 涉及模块 / 功能：MySQL 迁移、JWT 鉴权、用户资料加载
- 相关文件 / 函数：`backend/internal/server/middleware/jwt_auth.go`、`backend/internal/service/user_service.go`、`backend/internal/repository/user_profile_identity_repo.go`、`backend/migrations/mysql/001_baseline.sql`
- 运行环境：dev
- 其他上下文：数据库中 `users.id=1` 存在且状态正常，但 `schema_migrations` 仅有 `001_baseline.sql`

## 5. 严重程度

**P1** — 登录后核心用户接口不可用，但登录入口本身仍可工作。

## 备注

补充对照发现：当前 MySQL baseline 相比历史手工 migration 还缺少多张运行时表，当前事故直接命中的是 `user_avatars` / `user_provider_default_grants` 这一组。
