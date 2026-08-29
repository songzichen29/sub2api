# 生产分支上游合并参考

## 本次记录

- 日期：2026-08-29
- 生产基线：merge/upstream-main-into-pgsql-mysql-20260426
- 生产基线提交：554a88f579e61fd29b144c80402675777c037417
- 源仓库：https://github.com/Wei-Shaw/sub2api
- 源分支：main
- 源提交：b5827cfd54d58c248a9480b800444d0b40f0c6ea
- 集成分支：integration/weishaw-v0.1.183-into-prod-20260829
- 源版本：0.1.183
- 生产基线版本：0.1.177

本次合并使用 Git 合并提交。冲突先按生产侧优先处理，再对业务、数据库和前端进行人工修复。生产分支原有未提交修改先保存到临时 stash，合并后恢复，内容包括 MySQL 批量 model_mapping 替换语义及对应测试。

## 合并规则

1. 生产分支是唯一部署基线；禁止直接把源仓库 main 当成生产分支。
2. 源仓库的 Grok/xAI 官方集成不进入生产。删除源仓库重新引入的 Grok/xAI 文件，并确认通用文件不会重新启用 Grok 路由。
3. 保留生产分支的 PostgreSQL/MySQL 双数据库结构、订阅、发票、余额、免费图片桥接、上海时区和其他本地业务改造。
4. PostgreSQL 代码不能直接作为 MySQL 生产代码：检查 $1、ANY、ILIKE、NULLS LAST、::jsonb、ON CONFLICT、RETURNING、UPDATE ... FROM 等语法。
5. 源仓库新增 PostgreSQL migration 必须转换成 MySQL migration，并使用生产已有编号继续递增；不得复用源仓库编号。
6. MySQL migration 必须幂等，新增列、索引、约束先检查 information_schema；不得修改已发布的历史 migration。
7. Ent schema、生成代码、PostgreSQL migration、MySQL migration、repository SQL 必须同时更新。
8. 前端新增或移动字段必须同时检查 types、API barrel、组件导入、页面状态、中文/英文 locale 和页面 key 测试。
9. 合并后先做无冲突标记检查，再做前端 typecheck/Vitest，最后由 GitHub Actions 执行 Go、MySQL integration、lint 和前端构建。
10. 发布前后检查版本号、Docker/Compose、migration 集合、生成静态资源和 release workflow 产物。

## 冲突记录

第一次合并统计：224 个冲突，其中 188 个内容冲突、36 个修改/删除冲突。

内容冲突集中在 backend/ent、backend/internal/config、backend/internal/handler、backend/internal/repository、backend/internal/service、frontend/src/components/account、frontend/src/i18n、frontend/src/views/admin、deploy 和 .github/workflows。自动合并结果以生产侧实现为基础，新增源功能逐项人工补回。

以下修改/删除冲突均按生产策略删除，主要是 Grok/xAI 或生产已重构的旧文件：

backend/internal/handler/admin/grok_oauth_handler.go
backend/internal/handler/grok_audio.go
backend/internal/handler/grok_media.go
backend/internal/handler/openai_gateway_credential_failover_test.go
backend/internal/pkg/apicompat/chatcompletions_x_search_test.go
backend/internal/pkg/xai/billing.go
backend/internal/pkg/xai/cli_identity_test.go
backend/internal/pkg/xai/models.go
backend/internal/pkg/xai/models_test.go
backend/internal/pkg/xai/oauth_test.go
backend/internal/service/channel_monitor_service_grok_test.go
backend/internal/service/grok_audio.go
backend/internal/service/grok_media.go
backend/internal/service/grok_observed_models.go
backend/internal/service/grok_quota_service_test.go
backend/internal/service/grok_stream_idle.go
backend/internal/service/grok_stream_idle_test.go
backend/internal/service/grok_upstream_errors_test.go
backend/internal/service/grok_upstream_failure.go
backend/internal/service/grok_upstream_failure_test.go
backend/internal/service/grok_upstream_headers.go
backend/internal/service/openai_embeddings_test.go
backend/internal/service/openai_gateway_grok.go
backend/internal/service/openai_gateway_grok_cache.go
backend/internal/service/openai_gateway_grok_cache_test.go
backend/internal/service/openai_gateway_grok_chat_bridge.go
backend/internal/service/openai_gateway_grok_chat_bridge_test.go
backend/internal/service/openai_gateway_grok_test.go
backend/internal/service/openai_gateway_grok_tool_protocol_test.go
backend/internal/service/openai_gateway_model_availability.go
backend/internal/service/openai_gateway_search_surcharge_test.go
backend/internal/service/openai_ws_forwarder_logutil.go
backend/internal/service/openai_ws_forwarder_v2.go
backend/internal/service/ratelimit_service_scheduling_threshold_test.go
frontend/src/components/account/__tests__/CreateAccountModal.grok.spec.ts
frontend/src/views/admin/__tests__/ChannelMonitorView.grok.spec.ts

新增的 Grok/xAI 文件也全部删除。为保持共享 OpenAI/Codex 代码可编译，保留 backend/internal/pkg/xai/compat.go 中性兼容层；它不提供 Grok 路由、模型或配额。

## 数据库改造记录

生产 MySQL migration 原有编号到 078。本次新增：

| MySQL migration | 内容 |
|---|---|
| 079_user_platform_quotas_cn_providers.sql | CN 平台配额约束 |
| 080_codex_fingerprint_seed.sql | OAuth Codex 指纹种子回填 |
| 081_channel_model_time_pricing.sql | 渠道模型分时定价 |
| 082_usage_log_effective_model_indexes.sql | 有效请求/上游模型生成列和索引 |
| 083_channel_monitor_quota_mode.sql | 配额监控模式、关联账号、快照和 CN provider |
| 084_composite_routes_cn_providers.sql | Composite 路由支持 CN provider |
| 085_channel_pricing_multipliers.sql | Fast/Flex 和区间倍率 |
| 086_plugins.sql | OAuth 出站插件安装与绑定表 |
| 087_usage_reasoning_and_public_groups.sql | 请求 reasoning effort 与公共分组访问限制 |

源仓库重复的 PostgreSQL 编号 225、226、231 未直接复制。MySQL migration 使用 information_schema 检查保证重复启动安全。

特别检查：

- channel_repo_pricing.go 的 MySQL SELECT/INSERT/UPDATE 已补齐 fast_multiplier、flex_multiplier、区间倍率和 time_pricing 列。
- 插件 repository 已改为 MySQL ? 占位符、事务行锁和 LAST_INSERT_ID，移除 ON CONFLICT、RETURNING 和 ::jsonb。
- 生产本地 model_mapping 批量替换逻辑已恢复，避免 JSON 递归合并残留旧模型。
- Ent schema 已补回 channel_monitor.check_mode/account_id、CN provider 枚举和 user.restrict_public_groups 字段。

## 国际化检查

重点检查 frontend/src/i18n/locales/zh/admin、frontend/src/i18n/locales/en/admin、zh/common.ts、en/common.ts、zh/dashboard.ts 和 en/dashboard.ts。

必须同时存在中文和英文 key；禁止页面直接显示新增英文 fallback。前端 vue-tsc 和关键 Vitest 是发布门槛。

## 构建与发布流程

1. 推送集成分支后，GitHub Actions CI workflow 必须通过 shell、Go unit/integration、前端 typecheck/Vitest 和 golangci-lint。
2. 发布使用 Release workflow：更新 backend/cmd/server/VERSION，构建前端、后端 Docker/多架构镜像和 GitHub Release。
3. 发布 tag 必须与 VERSION 一致，生产环境使用 MySQL migration 集合。
4. 发布后检查 Actions artifact、GHCR 镜像、release notes 和 migration 文件。
5. 发布后再次对比生产基线到集成提交，确认没有 Grok/xAI 文件、冲突标记、丢失生产专用目录和配置。

## 容易出现的问题

- 把 origin/main 或上游 main 误当成生产分支，导致 MySQL/订阅魔改丢失。
- 只解决 Git 冲突，不检查自动合并结果，导致 Ent schema 与生成代码不一致。
- 把 PostgreSQL SQL 直接放入 MySQL repository 或 migration。
- 迁移编号重复，或新增 PostgreSQL migration 没有 MySQL 对应文件。
- 删除 Grok 文件后，通用 OpenAI 文件仍引用 internal/pkg/xai，导致 CI 编译失败。
- 前端组件被生产版本覆盖，页面模板仍引用上游状态/函数，导致 typecheck 失败。
- 只补中文 locale，英文页面显示原始 key，或 API barrel 没有导出新模块。
- 修改已上线 migration，触发 checksum mismatch。
- 只看本地构建，不看 GitHub Actions 的 Go 版本、pnpm、Docker 和 release workflow 环境差异。
- 发布后只检查 tag 成功，没有检查镜像实际包含的 migration、静态资源和版本文件。

## 当前验证记录

- 合并冲突标记：已清零。
- 前端 pnpm install --frozen-lockfile：通过。
- 前端 vue-tsc --noEmit：通过。
- 关键前端 Vitest：136 tests 通过。
- 本地 Go：宿主没有 Go；Docker golang:1.27.0 已尝试编译，曾发现缺失 go.sum 和 Grok 过滤后的共享引用，已补 go.sum 和中性兼容层。完整 Go 测试以 GitHub Actions 为准。
- GitHub Actions：已运行但未通过。提交 8df9a5d7d 的 CI/Security Scan 失败；随后重做合并提交 a7bc9e073 的 CI/Security Scan 仍失败。前端和 shell job 曾通过，backend Go test、golangci-lint 或 govulncheck 未通过，因此尚未进入生产分支和发布阶段。
