# 上游 v0.2.20 合并记录（2026-11-07）

## 范围与版本

- 原生产分支：`merge/upstream-main-into-pgsql-mysql-20260426`。
- 开始时本地提交：`8b5ddf793613bdf7743b5115e8b777f1b19e72ad`。
- 原上游基线：v0.2.17，`060d0160417782cd962a0c7f460f6cd4565d4916`。
- 本次上游目标：v0.2.20，`6b4b37db43443b5e88e424fe3c69602cc3810dfb`，区间内 228 个提交（含合并提交）。
- 上游合并提交：`c1a06214d`。
- 验证期间生产分支新增 `1fa16bb9e`、`46be4a401` 两笔接入文档提交，也纳入最终结果；保留公共接入文档、站内布局、导航及同源嵌入规则。
- 本次仅完成本地合并，不推送、不部署、不修改生产数据库。

上游发布说明：[v0.2.18](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.18)、[v0.2.19](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.19)、[v0.2.20](https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.20)。

## 上游主要变化

| 领域 | 本次更新 |
| --- | --- |
| 管理权限 | 新增受限管理员 API 密钥，可按功能权限、分组、平台、账号类型限制资源；补齐响应脱敏、轮换及并发撤权保护。 |
| MCP | 新增管理 MCP 和 OAuth 授权流程，包括 PKCE、授权同意、令牌轮换、撤销和重放检测。未配置 `MCP_PUBLIC_URL` 时不开启。 |
| 用户管理 | 新增注册邮箱黑名单、邮箱别名匹配和批量停用；停用时同时撤销 MCP 授权。 |
| 图片 | 新增按张计费、图片尺寸档位路由、GPT Image 2.6 支持，以及图片上传和流式错误处理修正。 |
| 模型与兼容 | 更新 GPT-6.1 Terra 等模型目录和价格；修正 Gemini/Antigravity 配额、工具结果图片、思考内容及媒体转换。 |
| 网关可靠性 | 增加请求体内存预算，限制异步任务和 I/O；修正 408/499、客户端断开后的用量记录、大请求解析与会话复用。 |
| 面板 | 用量导出改为分页和重试，增加受限密钥与 MCP 管理界面。 |

需要注意的默认行为：请求体内存预算默认 1 GiB，相关读写超时默认 120 秒；Gemini/Antigravity 分组的图片开关会由上游迁移开启，Composite 分组仍需显式启用。新建 Gemini 系分组也遵循上游默认启用图片的行为。已有图片批量开关及平台约束保留。

## 魔改保护与冲突处理

合并在独立 worktree `D:\data\sub2api-integration-v0.2.20` 进行。原提交由 `codex/backup-before-upstream-0.2.20-20261107` 保存，原工作区的 `.codex-qqbot-vision-4e50251/` 保留。

旧 v0.2.17 合并使用了以 v0.2.4 为父节点的快照提交 `e399f056a`。核对官方 v0.2.17 后，确认运行源码一致，差别为模式、换行和少量文档/脚本。先用 `9e33e8fff`、`3915fb1bc` 接回官方祖先，再合并 v0.2.20，把错误合并基线带来的 293 处冲突降到 50 处，并使后续更新可以使用真实的共同祖先。

相对官方 v0.2.17，开始时分支独有 847 个文件。上游合并中 845 个保持原样，只有以下两个独有文件发生兼容性调整：

- `backend/internal/handler/request_body_read_error.go`：保留原请求错误识别，同时接入上游读取进度及内存预算处理。
- `backend/internal/server/routes/account_import_templates_route_test.go`：测试注册调用补齐新增 step-up 参数。

该统计只证明独有文件的保留情况；双方共同修改的文件另行审查，不能据此声称所有功能均完成端到端验证。QQ 机器人目录未改动；唯一删除的既有文件为上游移除的合作方图片 `assets/partners/logos/apimart.jpg`。

共同修改文件中重点保留了：

- 自定义余额预占/扣费、透支、订阅限制及订阅时间操作。
- QQ 机器人路由、依赖注入、后台任务和关闭钩子；模型质量相关钩子。
- 账号标签、筛选/排序、自定义平台、语言提示及界面选项。
- previous-response 会话亲和、CCH 签名与 system-reminder 处理、5/10/15 秒网络退避，同时接入上游冷却禁用开关。
- 用户消息串行队列依赖的已解析消息字段，并保证请求复用时各次尝试的数据隔离。
- 原模型列表别名接口；受限管理员密钥对 QQ 管理及未授权旧接口仍默认拒绝。

前端语言文件按键合并，保留双方新增文案；最后两笔接入文档提交与 MCP 导航的重叠处同时保留。

## MySQL 兼容处理

保留上游 PostgreSQL 路径，并新增 `backend/migrations/mysql/111_upstream_v0_2_20.sql`。该迁移包含图片计费字段、受限管理员密钥、MCP 授权/凭据表以及代理分配锁行。原有迁移未改写；文件为 UTF-8 无 BOM，首字节为 `55 50 44 41 54 45 20 60`。

新增 SQL 参数适配不仅替换 `$n`，还同步重排和重复绑定参数，避免轮换、CAS 和撤销写错参数。MySQL 分支分别适配 JSON 投影、邮箱别名查询、随机代理、Grok 模型 CAS 以及事务锁。受限账号查询仍只读取授权字段，不以加载完整凭据作为兼容捷径。

真实 MySQL 检查发现并修复了两项额外差异：邮箱查询的排序规则冲突，以及 MySQL 默认返回“实际变更行数”导致重复 touch/撤销被误认为记录不存在。最空闲代理分配使用事务行锁与当前读，账号修改使用账号和关联行锁，不使用会泄漏到连接池的会话级锁。

## 验证结果与边界

已通过的检查：

- 前端全量测试：428 个文件，3469 项测试；类型检查和生产构建通过。
- 接回并行的接入文档提交后，导航、路由、i18n、首页及文档页面共 24 个文件、175 项测试复测通过；后端文档嵌入安全头专项测试通过。
- Ent、Wire 生成通过；后端 `go build -p 2 -tags=embed -o bin/sub2api-merge-check.exe ./cmd/server` 通过。
- 独立本地 MySQL 8.4.4：迁移 111 在最小相关表结构上重复执行；受限密钥 CRUD/CAS/轮换/撤销；MCP 授权码交换、刷新令牌重放、重复撤销；邮箱停用联动；敏感字段投影、Grok CAS、并发代理分配和账号事务锁/回滚通过。测试不连接生产库。
- Repository 针对本次 SQL、余额预占/扣费、MySQL 配额、受限密钥和 MCP 的回归通过。
- 路由包完整 `-tags=unit` 测试通过，包括受限权限、MCP OAuth HTTP 流程及原接口保护。
- Service 针对请求体、会话摘要、图片、DeepSeek、订阅、扣费、队列、受限密钥、MCP、UTF-16 指纹和冷却开关的回归通过。

不能声称后端全量测试通过：

- 原分支已有测试编译问题。为运行 Service 回归，仅在临时 `go -overlay` 中排除 6 个原样保留的测试文件：`admin_proxy_credentials_update_test.go`、`backup_recovery_test.go`、`backup_retention_test.go`、`channel_monitor_endpoint_test.go`、`payment_order_provider_snapshot_test.go`、`payment_recharge_bonus_test.go`。它们涉及既有指针类型、缺失备份/监控/返利函数及支付参数不匹配；未为通过测试改写这些业务功能。
- 完整后端测试还暴露既有渠道定价/统计 SQL 断言、代理凭据更新、JWT 中文提示与英文断言差异，以及未接线的账号测试日志断言。Grok/Seedance 三组失败已在合并前工作区复现。
- Service 定向回归跳过了原样保留的 `TestAccountTestService_OpenCodeGoDeepSeekFlashUsesChatCompletions`：该测试要求旧账号测试路径设置尚未接入的 OpenCode session header。
- 非 unit 的 Service 全量尝试出现分配次数和两项刷新时序断言失败；上述测试单独复测通过，相关刷新实现与原分支一致。
- 本机 golangci-lint v2.9.0 使用 Go 1.26.3 编译，拒绝分析项目 Go 1.27.3；未将此项标为通过。
- 未做实际外部模型调用、QQ 群发言、线上用户操作或生产数据库升级验证。

测试日志保存在原工作区的 `.codex/merge-v0.2.20-tools/`（未提交）。AOCI 按仓库规则仅进行最终维护检查；该源码合并记录不代表索引已经对齐。
