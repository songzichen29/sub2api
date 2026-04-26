# PostgreSQL 替换为 MySQL 改造计划

## 1. 目标

本计划用于指导 Sub2API 从当前的 PostgreSQL 单栈实现，迁移为 MySQL 单栈实现。

本次选择的路线是：

- 采用方案 A：直接替换 PostgreSQL，不保留 PostgreSQL 运行支持
- 目标是最终仓库、部署模板、测试基座、文档说明统一以 MySQL 为默认且唯一主数据库

不在本计划范围内的内容：

- PostgreSQL 与 MySQL 双栈长期并存
- 兼容历史 PostgreSQL 运维工具链的长期保留
- 一次性覆盖所有历史数据库的自动在线迁移工具实现细节

## 2. 当前现状摘要

当前仓库对 PostgreSQL 存在明显强绑定，主要体现在：

1. 连接与初始化层固定使用 `lib/pq`、`dialect.Postgres` 与 `sql.Open("postgres", ...)`
2. 安装向导、自动安装、Docker Compose、示例配置均默认 PostgreSQL
3. 迁移系统依赖 PostgreSQL 参数风格与 advisory lock
4. `backend/migrations/` 下已有 161 个 SQL migration，包含大量 PostgreSQL 专有语法
5. Repository 层存在大量手写 PostgreSQL SQL（`ANY`、`pq.Array`、`ILIKE`、`RETURNING`、`jsonb`、`::type`、`date_trunc`、`FOR UPDATE SKIP LOCKED` 等）
6. 备份恢复链路直接依赖 `pg_dump` / `psql`
7. 集成测试与开发文档以 PostgreSQL 为唯一主数据库

因此，这不是“替换驱动”级别工作，而是数据库方言迁移工程。

## 3. 总体改造原则

1. 先建立 MySQL 基础设施，再逐模块替换业务实现
2. 先确保“可启动 + 可迁移 + 可通过核心测试”，再处理长尾 SQL 优化
3. 优先消除 PostgreSQL 运行依赖，不追求保留 PostgreSQL 兼容代码
4. 对所有手写 SQL 明确进行逐类替换，避免隐性方言残留
5. 数据迁移方案与代码迁移方案分离推进，避免一次性耦合过深

## 4. 模块拆分改造计划

---

### 模块 A：数据库基础设施与配置层

#### 目标

让系统以 MySQL 驱动、DSN、连接池配置启动，并移除 PostgreSQL 运行前提。

#### 影响范围

- `backend/internal/config/config.go`
- `backend/internal/repository/ent.go`
- `backend/internal/setup/setup.go`
- `backend/internal/setup/cli.go`
- 所有依赖 `sql.Open("postgres", ...)` 或 `dialect.Postgres` 的初始化路径

#### 主要改造项

1. 替换数据库驱动依赖
   - 移除 `github.com/lib/pq`
   - 引入 MySQL 驱动
2. 重写 `DatabaseConfig.DSN()` / `DSNWithTimezone()`
3. 将 Ent 初始化从 `dialect.Postgres` 改为 `dialect.MySQL`
4. 改造 setup 阶段数据库探测与建库逻辑
5. 统一环境变量与默认值语义（host/port/user/password/dbname）
6. 审查时区、连接池、字符集、`parseTime` 等 MySQL 必要参数

#### 风险点

- MySQL 时区与时间解析行为与 PostgreSQL 不同
- `TimeZone=...` 这类 PostgreSQL DSN 参数不可直接复用
- 字符集/排序规则不一致可能影响唯一约束与模糊匹配

#### 验收标准

- 服务可通过 MySQL 正常初始化连接
- setup 向导可成功探测并初始化空数据库
- 应用主进程可在本地 MySQL 下完成启动

---

### 模块 B：迁移体系重建

#### 目标

提供一套可在 MySQL 上完整建库的迁移体系，替代当前 PostgreSQL-only migration 集合。

#### 影响范围

- `backend/migrations/*.sql`
- `backend/internal/repository/migrations_runner.go`
- 可能涉及 Atlas 基线逻辑

#### 主要改造项

1. 重建迁移元数据表 DDL
   - `TIMESTAMPTZ`、`TEXT[]` 等 PostgreSQL 类型改写
2. 替换 migration runner 中的 PostgreSQL 参数风格与锁机制
   - advisory lock 改为 MySQL 可实现的全局互斥机制（例如 named lock 或迁移表行锁方案）
3. 处理 `*_notx.sql` 机制与 MySQL DDL 事务差异
4. 重写 161 个 migration 的落地策略

#### 推荐实施策略

不建议机械逐条翻译全部历史 migration；推荐采用：

- 先整理一份 **MySQL 基线建库脚本**，覆盖当前最新 schema
- 再将仍需保留的增量迁移，按 MySQL 方言重写为新的迁移序列

这样比逐条翻译历史 PostgreSQL 演进路径更稳，也更省工。

#### 高风险语法清单

- `BIGSERIAL`
- `jsonb`
- `timestamptz`
- `::type`
- `CREATE OR REPLACE FUNCTION`
- `pg_indexes` / `pg_constraint`
- `GIN` / `TRGM`
- `PARTITION`
- `ON CONFLICT`
- `RETURNING`

#### 验收标准

- 空 MySQL 数据库可通过 migration 完整建库
- migration runner 支持重复启动幂等
- schema_migrations 校验逻辑在 MySQL 下可工作

---

### 模块 C：Ent Schema 与代码生成层

#### 目标

消除 Ent schema 中的 PostgreSQL 类型假设，使其可在 MySQL 方言下正确生成与运行。

#### 影响范围

- `backend/ent/schema/**/*.go`
- `backend/ent/**/*.go`（生成产物）

#### 主要改造项

1. 审查所有 `SchemaType(map[string]string{dialect.Postgres: ...})`
2. 为时间、decimal、text、json 等字段补齐 MySQL 侧定义
3. 重新生成 ent 代码
4. 检查 Ent 生成代码中仅支持 SQLite/PostgreSQL 的路径

#### 风险点

- 时间字段精度、NULL 语义、默认值表达式可能变化
- 部分 upsert / conflict helper 在 MySQL 方言下生成行为不同

#### 验收标准

- Ent 代码可重新生成
- 编译通过
- 核心实体 CRUD 在 MySQL 下正常工作

---

### 模块 D：Repository 手写 SQL 改造

#### 目标

逐步清除 PostgreSQL 专有 SQL，保证业务仓储层可在 MySQL 上运行。

#### 影响范围

- `backend/internal/repository/**/*.go`

#### 主要改造项分组

1. 数组与批量参数
   - `pq.Array`
   - `ANY($1)`
   - `unnest(...)`
2. 文本检索
   - `ILIKE`
3. 类型转换
   - `::date`
   - `::json`
   - `::timestamptz`
4. 返回更新结果
   - `RETURNING`
5. 锁与并发控制
   - `FOR UPDATE SKIP LOCKED`
   - advisory lock
6. 时间聚合
   - `date_trunc`
   - `TO_CHAR`
7. JSON 查询与更新
   - `jsonb_build_object`
   - `jsonb_set`
   - `@>`
   - `->` / `->>`
8. PostgreSQL catalog 访问
   - `pg_indexes`
   - `pg_constraint`

#### 推荐执行顺序

1. 先处理启动期必须路径
2. 再处理写路径与事务路径
3. 最后处理报表、聚合、后台任务、运维统计等非启动关键路径

#### 特别关注文件

- `usage_log_repo.go`
- `usage_cleanup_repo.go`
- `user_group_rate_repo.go`
- `dashboard_aggregation_repo.go`
- `ops_repo.go`
- `account_repo.go`
- `group_repo.go`
- `channel_*_repo.go`

#### 验收标准

- Repository 层不再依赖 `lib/pq`
- 核心读写链路在 MySQL 下可跑通
- 高风险 SQL 已有对应替代实现与回归测试

---

### 模块 E：服务层与数据库能力适配

#### 目标

确保 service 层没有建立在 PostgreSQL 特有行为上的隐含假设。

#### 影响范围

- `backend/internal/service/**/*.go`

#### 主要改造项

1. 清理 `client.Driver().Dialect() == dialect.Postgres` 的特判逻辑
2. 审查依赖 `RETURNING` 返回值的事务服务流程
3. 审查 JSON 字段、统计聚合、锁语义差异对业务结果的影响
4. 重新定义“非 PostgreSQL 时禁用”的功能策略

#### 特别关注点

- 仪表盘预聚合
- usage cleanup 任务调度
- payment / billing 事务一致性
- auth identity 相关并发保护

#### 验收标准

- 关键服务不因 MySQL 方言差异导致功能降级或静默禁用
- 服务层测试可在 MySQL 下通过

---

### 模块 F：备份恢复与数据管理

#### 目标

将数据库备份恢复从 PostgreSQL 工具链切换到 MySQL 工具链。

#### 影响范围

- `backend/internal/repository/backup_pg_dumper.go`
- `backend/internal/service/backup_service.go`
- 后台 data management 相关 handler / API / 前端文案

#### 主要改造项

1. `pg_dump` / `psql` 替换为 MySQL 备份恢复工具
2. 调整备份文件格式与失败提示
3. `backup_type=postgres`、`postgres_profile_id` 等命名统一重构
4. 更新前端和 API 文案，避免继续暴露 PostgreSQL 术语

#### 风险点

- MySQL dump 在事务一致性、锁表与恢复行为上和 PostgreSQL 不同
- 现有备份记录元数据可能需要调整字段语义

#### 验收标准

- 可成功执行数据库备份
- 可在测试环境执行恢复并完成基本校验

---

### 模块 G：安装、部署、容器与运维模板

#### 目标

让默认部署方式全面改为 MySQL。

#### 影响范围

- `deploy/docker-compose*.yml`
- `deploy/install*.sh`
- `deploy/README.md`
- `deploy/DOCKER.md`
- `.env.example`
- `config.example.yaml`

#### 主要改造项

1. Compose 服务从 `postgres` 改为 `mysql`
2. 调整环境变量命名与默认端口
3. 修改健康检查命令
4. 审查 volume、初始化参数、字符集、时区
5. 更新安装脚本与自动部署脚本

#### 验收标准

- 默认 docker compose 可拉起 MySQL + Redis + sub2api
- 首次启动可自动完成数据库初始化

---

### 模块 H：测试体系与 CI

#### 目标

让单元测试、集成测试、CI 基线切换到 MySQL。

#### 影响范围

- `backend/internal/repository/integration_harness_test.go`
- 所有 integration tests
- `.github/workflows/backend-ci.yml`
- 可能涉及本地开发文档中的测试说明

#### 主要改造项

1. testcontainers 从 postgres 模块改为 mysql 容器
2. 调整集成测试建库、等待、DSN、清理逻辑
3. 修复依赖 PostgreSQL SQL 行为的测试
4. 重跑核心 unit/integration 组合

#### 验收标准

- 后端 unit tests 通过
- 后端 integration tests 在 MySQL 下通过
- CI 能稳定复现

---

### 模块 I：前端、接口契约与文案

#### 目标

清理面向用户/管理员暴露的 PostgreSQL 命名和假设。

#### 影响范围

- `frontend/src/api/**/*.ts`
- `frontend/src/i18n/locales/*.ts`
- setup / data management / ops 页面
- README / DEV_GUIDE / docs

#### 主要改造项

1. setup 接口文案从 PostgreSQL 改为 MySQL
2. data management 中 `postgres` 相关类型名、字段名、按钮文案调整
3. 更新 README 技术栈和部署说明
4. 更新开发文档中的数据库说明与命令示例

#### 验收标准

- 前端不再向用户暴露 PostgreSQL 专用术语
- 文档与实际部署方式一致

---

### 模块 J：数据迁移方案（实施前置）

#### 目标

为已有 PostgreSQL 线上数据迁移到 MySQL 提供独立方案。

#### 说明

这是方案 A 真正落地时不可回避的前置条件，但建议与代码改造并行设计、独立验证。

#### 主要内容

1. 明确迁移对象
   - 核心业务表
   - 配置表
   - 审计/日志表
2. 明确字段映射
   - `jsonb -> json`
   - `bigserial -> bigint auto_increment`
   - `timestamptz -> datetime/timestamp`
3. 明确迁移策略
   - 冷迁移
   - 双写过渡（若必须）
   - 导出/导入脚本
4. 明确校验策略
   - 行数校验
   - 抽样校验
   - 核心业务数据核对

#### 验收标准

- 有明确、可执行、可回滚的数据迁移手册

## 5. 推荐实施顺序

建议分 5 个阶段推进：

### 阶段 1：基础设施切换

- 模块 A
- 模块 C
- 模块 G（最小启动集）

目标：应用能连 MySQL，主程序具备启动条件。

### 阶段 2：迁移体系与建库能力

- 模块 B

目标：空库可通过 MySQL migration 完整建库。

### 阶段 3：核心业务读写链路替换

- 模块 D（核心路径）
- 模块 E（核心路径）

目标：登录、用户、分组、API Key、计费、网关核心链路跑通。

### 阶段 4：运维与附加能力替换

- 模块 D（长尾路径）
- 模块 E（长尾路径）
- 模块 F
- 模块 I

目标：管理后台、报表、备份恢复、文案一致性收尾。

### 阶段 5：测试闭环与数据迁移演练

- 模块 H
- 模块 J

目标：形成可上线切换方案。

## 6. 粗略工作量评估

以 1 名熟悉 Go / SQL / Ent 的工程师估算：

| 模块 | 工作量级别 | 说明 |
|------|------------|------|
| A 基础设施与配置 | 中 | 驱动、DSN、setup、初始化 |
| B 迁移体系重建 | 高 | 161 个 migration 的策略是最大头 |
| C Ent schema/codegen | 中 | 范围可控，但需细查 |
| D Repository SQL | 高 | 多文件、多类 PostgreSQL 专有语法 |
| E 服务层适配 | 中高 | 依赖 D 的结果 |
| F 备份恢复 | 中 | 工具链替换 + 接口命名调整 |
| G 部署运维模板 | 中 | compose、脚本、示例配置 |
| H 测试与 CI | 中高 | 集成测试与回归成本高 |
| I 前端与文档 | 中 | 改动面广但技术风险低 |
| J 数据迁移方案 | 中高 | 与上线策略强相关 |

总体判断：

- 这是一次 **中大型改造**
- 若按“完成替换并具备上线基础”估算，通常不是几天内完成的任务

## 7. 关键风险

1. 历史 migration 逐条翻译成本过高
2. 手写 SQL 的 PostgreSQL 依赖散落且隐蔽
3. MySQL 在 JSON、锁、返回值、时间函数上的语义差异可能带来业务回归
4. 报表/聚合/后台任务属于高风险长尾区域
5. 数据迁移若没有单独演练，最终上线风险很高

## 8. 建议的实施前确认项

在正式进入代码改造前，建议先确认以下事项：

1. 是否接受“不保留 PostgreSQL 运行支持”
2. 是否接受“重建 MySQL 基线迁移”而不是逐条翻译全部历史 migration
3. 是否接受“数据迁移方案独立成一个后续子任务”
4. 首批必须跑通的核心链路范围是否定义为：
   - 系统启动
   - setup
   - 登录/用户/分组/API Key
   - 网关主链路
   - 核心计费与 usage log

## 9. 下一步建议

若确认按本计划实施，建议下一步进入：

1. 先输出“阶段 1 + 阶段 2”的具体文件级任务清单
2. 再按最小可启动路径开始替换基础设施与 migration 基线
