---
doc_type: issue-fix
issue: 2026-04-25-mysql-auth-schema-gaps
status: confirmed
related: [mysql-auth-schema-gaps-report.md, mysql-auth-schema-gaps-analysis.md]
tags: [mysql, migrations, auth, schema]
---

# MySQL 迁移遗漏导致鉴权异常 Fix Note

## 1. 本次修复

### 1.1 直接修复当前 401 链路

- 新增 `backend/migrations/mysql/002_auth_profile_followup.sql`
  - 补齐 `user_provider_default_grants`
  - 补齐 `user_avatars`
  - 补齐相关 auth source 默认 settings 种子数据
- 调整 `backend/internal/repository/user_profile_identity_repo.go`
  - `GetUserAvatar()` 在 `user_avatars` 缺表时降级返回 `nil`，不再打断鉴权热路径
- 调整 `backend/internal/server/middleware/middleware.go`
  - 新增 `AbortForUserLookupError()`，仅对真正的 `ErrUserNotFound` 返回 401
- 调整 `backend/internal/server/middleware/jwt_auth.go`
  - 使用新的用户读取错误映射逻辑
- 调整 `backend/internal/server/middleware/admin_auth.go`
  - 使用新的用户读取错误映射逻辑

### 1.2 完整补齐 MySQL raw/manual schema parity

- 新增 `backend/migrations/mysql/003_runtime_schema_parity.sql`
- 覆盖对象：
  - 审计/账务：
    - `orphan_allowed_groups_audit`
    - `billing_usage_entries`
    - `usage_billing_dedup`
    - `usage_billing_dedup_archive`
  - 渠道/定价：
    - `channel_pricing_intervals`
    - `channel_account_stats_pricing_rules`
    - `channel_account_stats_model_pricing`
    - `channel_account_stats_pricing_intervals`
    - `channel_monitor_aggregation_watermark`
  - 认证迁移报告：
    - `auth_identity_migration_reports`
  - 用户扩展：
    - `user_group_rate_multipliers`
    - `user_affiliates`
    - `user_affiliate_ledger`
- 同时补齐了当前代码直接依赖、但 baseline 未包含的列：
  - `channels.model_mapping`
  - `channels.billing_model_source`
  - `channels.restrict_models`
  - `channels.features`
  - `channels.features_config`
  - `channels.apply_pricing_to_account_stats`
  - `channel_model_pricing.platform`
  - `channel_model_pricing.billing_mode`
  - `channel_model_pricing.per_request_price`
  - `usage_logs.image_output_tokens`
  - `usage_logs.image_output_cost`
  - `usage_logs.reasoning_effort`
  - `usage_logs.openai_ws_mode`
  - `usage_logs.request_type`
  - `usage_logs.service_tier`
  - `usage_logs.inbound_endpoint`
  - `usage_logs.upstream_endpoint`
  - `usage_logs.account_stats_cost`
- 额外补上关键索引/约束：
  - `idx_channel_model_pricing_platform`
  - `idx_usage_logs_request_type_created_at`
  - `idx_usage_logs_service_tier_created_at`
  - `idx_payment_audit_logs_order_action_uniq`

### 1.3 migration 入口修正

- 调整 `backend/migrations/migrations.go`
  - 根目录 `FS` 保留给历史 migration regression test 读取
  - 运行时新增 `migrations.MySQLFS`
- 调整 `backend/internal/repository/ent.go` / `migrations_runner.go`
  - 启动迁移显式使用 `migrations.MySQLFS`

### 1.4 验证期间顺手修复的编译阻塞

- 调整 `backend/internal/repository/ops_repo_dashboard.go`
  - 修掉 `fmt.Sprintf` 无格式占位符导致的构建失败，避免 repository 包无法通过基本编译验证

### 1.5 第二轮清理：运行时代码 PostgreSQL 方言残留

- 调整 `backend/internal/repository/usage_log_repo.go`
  - 去掉 raw SQL 中残留的 `ON CONFLICT` / `RETURNING`
  - 批量/单条写入统一改为 MySQL `ON DUPLICATE KEY UPDATE id = id`
  - `resolveModelDimensionExpression()` / `resolveEndpointColumn()` 改为 MySQL `CONCAT(...)`
- 调整 `backend/internal/repository/ops_repo_alerts.go`
  - `CreateAlertRule` / `UpdateAlertRule` / `CreateAlertEvent` / `CreateAlertSilence` 改为 `Exec + LastInsertId/RowsAffected + SELECT`
  - 修正 `UpdateAlertRule` / `UpdateAlertEventStatus` / `UpdateAlertEventEmailSent` 的参数顺序错误
  - `dimensions->>'...'` 改为 `JSON_UNQUOTE(JSON_EXTRACT(...))`
- 调整 `backend/internal/repository/scheduled_test_repo.go`
  - 去掉 `RETURNING`
  - 修正 `Update` / `UpdateAfterRun` 的参数顺序错误
- 调整 `backend/internal/repository/ops_repo_dashboard.go`
  - 去掉 `percentile_cont ... FILTER (...)`
  - 改为拉取样本后在应用层计算 P50/P90/P95/P99
- 调整 `backend/internal/repository/ops_repo_trends.go`
  - `COUNT(*) FILTER (WHERE ...)` 改为 `SUM(CASE WHEN ... THEN 1 ELSE 0 END)`
- 调整 `backend/internal/repository/account_repo.go`
  - `extra->>'crs_account_id'` 改为 MySQL JSON 提取语法
- 调整 `backend/internal/repository/group_repo.go` / `ops_repo.go` / `ops_repo_dashboard.go` / `ops_repo_preagg.go` / `ops_repo_request_details.go` / `ops_repo_trends.go` / `usage_billing_repo.go`
  - 将 raw SQL 中裸写的 `groups` 表统一改为 MySQL 安全引用 `` `groups` ``
  - 顺手修正 `ops_repo_trends.go` 两处查询参数个数不匹配

### 1.6 第三轮清理：低版本 MySQL migration 兼容

- 调整 `backend/migrations/mysql/003_runtime_schema_parity.sql`
  - 去掉 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...` 多列写法
  - 改为 `information_schema.columns` + `PREPARE/EXECUTE` 的逐列幂等补列
  - 兼容当前用户环境中已验证不支持该语法的 MySQL 版本
- 调整 `backend/migrations/auth_identity_payment_migrations_regression_test.go`
  - 断言改为校验逐列存在性检测逻辑，而不是旧的 `ADD COLUMN IF NOT EXISTS` 文本

### 1.7 第四轮清理：本地开发环境数据目录 / 日志路径误判

- 调整 `backend/internal/setup/setup.go`
  - `GetDataDir()` 先检查当前工作目录是否已有 `config.yaml` / `.installed`
  - 避免本地运行时因为系统里恰好存在 `/app/data` 而被误判成 Docker 数据目录
  - `GetConfigFilePath()` / `GetInstallLockPath()` 改为 `filepath.Join(...)`
- 调整 `backend/internal/config/config.go`
  - 配置加载优先读取当前工作目录中显式存在的 `config.yaml`
  - 避免本地开发启动时被 `/app/data/config.yaml` 抢占优先级
- 调整 `backend/internal/pkg/logger/options.go`
  - 本地项目目录存在 `config.yaml` / `.installed` 时，默认日志路径回落到 `logs/sub2api.log`
  - 避免无 `DATA_DIR` 时仍默认写入容器路径 `/app/data/logs/sub2api.log`

## 2. 验证

已完成：

- `go test ./migrations ./internal/server/middleware -tags unit`
  - 结果：通过
- `go test ./internal/repository -run '^$'`
  - 结果：通过（验证 repository 包可编译）
- `go test ./internal/... -run '^$'`
  - 结果：通过（后端全部 internal 包编译通过）
- `go test ./internal/repository -tags unit -run 'UsageLogRepository|BuildRequestTypeFilterConditionLegacyFallback|BuildUsageLog|ExecUsageLogInsertNoResult|PrepareUsageLogInsert|CoalesceTrimmedString|ResolveEndpointColumn|ResolveModelDimensionExpression'`
  - 结果：通过（覆盖本轮 usage log / MySQL SQL 方言相关单元测试）
- `go test ./internal/repository -tags unit`
  - 结果：通过（repository 包完整单元测试通过）
- `go test ./internal/setup ./internal/config ./internal/pkg/logger`
  - 结果：通过（覆盖本地配置目录优先级 / 日志路径 / setup 判定修复）
- `go test ./... -run '^$'`
  - 结果：通过（全仓库编译通过）
- 新增/覆盖的验证点：
  - MySQL `002` follow-up migration 包含 `user_provider_default_grants` / `user_avatars`
  - MySQL `003` follow-up migration 包含剩余 raw/manual 表与关键列
  - MySQL `003` follow-up migration 不再依赖 `ADD COLUMN IF NOT EXISTS`
  - JWT 中间件在用户真实不存在时仍返回 `USER_NOT_FOUND`
  - JWT / Admin JWT 在用户读取发生内部错误时返回 `INTERNAL_ERROR`
  - runtime 代码已复扫：执行 SQL 不再残留 `ON CONFLICT` / `RETURNING` / `FILTER (WHERE ...)` / `percentile_cont` / `->>`
  - raw SQL 已复扫：不再残留未转义的 `groups` 表引用
  - 当前目录已有 `config.yaml` / `.installed` 时，不再误进入 setup wizard
  - 当前目录已有项目配置时，默认日志路径回到 `backend/logs/sub2api.log`

未完成：

- 未在可稳定访问目标 MySQL 的网络环境里完成整段运行验证
- 未在真实运行库上逐一回放 `/api/v1/auth/me`、`/api/v1/announcements`、`/api/v1/usage`

## 3. 结果判断

本次 “pgsql -> mysql 手工迁移遗漏” 已从单点修补提升为完整的 runtime schema parity 修复：

1. 直接导致 401 的 `user_avatars` 缺表已补齐
2. 同一类 raw/manual 表遗漏已批量补齐
3. 当前代码直接查询但 baseline 缺失的关键列已补齐
4. 运行时 raw SQL 中残留的 PostgreSQL 方言已清理到 MySQL 兼容实现
5. 用户读取错误不再被误伪装成 `USER_NOT_FOUND`
6. 启动阶段不再因 `/app/data` 优先级误判而错误进入 setup wizard
7. 低版本 MySQL 上 `003_runtime_schema_parity.sql` 不再因 `ADD COLUMN IF NOT EXISTS` 语法失败

## 4. 剩余风险

这轮修复聚焦在“当前代码仍直接使用的 raw/manual schema 对象”。

未纳入本次 follow-up 的对象主要有两类：

- 已被后续 migration 删除的历史表（如 `sora_accounts` / `sora_generations`），不应重新创建
- 纯 PostgreSQL 语义、当前 MySQL 运行路径未直接依赖的历史数据迁移逻辑（如部分一次性 backfill / CHECK / comment）

如果后续要把“历史 migration 语义 100% 等价搬迁”也做完，建议另开一轮 schema parity / data parity issue，单独处理数据回填与兼容约束。
