# MySQL 替换实施清单（阶段 1 + 阶段 2）

## 1. 适用范围

本清单仅覆盖：

- 阶段 1：基础设施切换
- 阶段 2：迁移体系与建库能力

目标是先让项目具备以下能力：

1. 后端可使用 MySQL 启动
2. 空 MySQL 数据库可完成 schema 初始化
3. 默认部署模板改为 MySQL

本清单不包含：

- Repository 业务 SQL 全量改造
- Service 层业务适配
- 前端文案收尾
- 数据迁移脚本

---

## 2. 阶段 1：基础设施切换

### 2.1 Go 依赖与数据库驱动

#### 文件

- `backend/go.mod`
- `backend/go.sum`

#### 任务

1. 移除 PostgreSQL 驱动依赖 `github.com/lib/pq`
2. 引入 MySQL 驱动依赖
3. 保留 SQLite 相关测试依赖不动

#### 验收

- `go mod tidy` 后依赖关系正确
- 不再存在运行期必须依赖 `lib/pq` 的路径

---

### 2.2 数据库配置模型与 DSN

#### 文件

- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`

#### 任务

1. 重写 `DatabaseConfig.DSN()`
2. 重写 `DatabaseConfig.DSNWithTimezone()`
3. 为 MySQL DSN 补齐必要参数，例如：
   - charset
   - parseTime
   - loc / timezone
4. 审核现有默认值：
   - 默认端口从 `5432` 改为 `3306`
   - 默认用户/密码/数据库名语义改为 MySQL 风格
5. 更新相关测试断言

#### 验收

- 配置测试可校验 MySQL DSN 生成结果
- DSN 中不再出现 PostgreSQL 专用参数格式

---

### 2.3 Ent 初始化与数据库连接入口

#### 文件

- `backend/internal/repository/ent.go`
- 如有必要：`backend/internal/service/wire.go`
- 如有必要：`backend/internal/handler/wire.go`

#### 任务

1. 将驱动副作用导入从 PostgreSQL 驱动切到 MySQL 驱动
2. 将 `entsql.Open(dialect.Postgres, dsn)` 改为 MySQL 方言
3. 确认连接池设置逻辑对 MySQL 仍成立
4. 保证启动期 migration 调用链不变，仅切换底层数据库类型

#### 验收

- 应用可在 MySQL 上完成 `InitEnt`
- 编译通过

---

### 2.4 安装向导与自动初始化

#### 文件

- `backend/internal/setup/setup.go`
- `backend/internal/setup/cli.go`
- `backend/internal/setup/setup_test.go`
- `frontend/src/api/setup.ts`

#### 任务

1. 将 setup 中的 `sql.Open("postgres", ...)` 改为 MySQL 驱动
2. 重写数据库探测与建库逻辑：
   - 不再查询 `pg_database`
   - 改为 MySQL 可用的 existence check
3. 调整数据库名/用户名校验是否仍适合 MySQL
4. CLI 提示词从 `PostgreSQL` 改为 `MySQL`
5. 更新 setup 测试与前端接口注释

#### 验收

- setup API 可成功测试 MySQL 连接
- CLI 安装流程在 MySQL 下可跑通

---

### 2.5 Ent Schema 的 MySQL 类型对齐

#### 文件

- `backend/ent/schema/**/*.go`

#### 首批重点文件

- `backend/ent/schema/user_subscription.go`
- 其他包含 `SchemaType(map[string]string{dialect.Postgres: ...})` 的 schema 文件

#### 任务

1. 搜索所有 `dialect.Postgres` 定制类型
2. 为时间、decimal、text、json 等字段补齐或替换为 MySQL 可用定义
3. 处理 `timestamptz`、`decimal(20,10)`、`text` 等方言差异
4. 重新生成 ent 代码

#### 验收

- `backend/ent/` 生成产物更新完成
- schema 编译通过

---

### 2.6 默认配置与部署模板最小启动集

#### 文件

- `deploy/docker-compose.yml`
- `deploy/docker-compose.local.yml`
- `deploy/docker-compose.dev.yml`
- `deploy/docker-compose.standalone.yml`
- `deploy/.env.example`
- `deploy/config.example.yaml`
- `deploy/README.md`
- `deploy/DOCKER.md`

#### 任务

1. 把数据库服务从 `postgres` 改为 `mysql`
2. 修改环境变量命名与默认值
3. 修改健康检查命令
4. 补齐 MySQL 容器启动参数（字符集、认证方式、时区视情况）
5. 保持 Redis 配置不变

#### 验收

- 默认 compose 能拉起 MySQL + Redis + 应用容器
- 文档中的默认数据库说明与实际一致

---

## 3. 阶段 2：迁移体系与建库能力

### 3.1 迁移执行器核心改造

#### 文件

- `backend/internal/repository/migrations_runner.go`

#### 任务

1. 重写 `schema_migrations` 表 DDL
2. 重写 `atlas_schema_revisions` 表 DDL
3. 将 PostgreSQL 参数风格 `$1/$2` 改为兼容 MySQL 的执行方式
4. 替换 advisory lock 机制
5. 审查 `*_notx.sql` 逻辑是否仍成立
6. 审查 checksum 记录与查询逻辑在 MySQL 下的兼容性

#### 验收

- migration runner 能在 MySQL 空库启动
- migration 元数据表可正确创建和查询

---

### 3.2 MySQL 基线迁移设计

#### 文件

- `backend/migrations/`

#### 建议产物

1. 一组新的 MySQL 基线建库 migration
2. 对应的迁移命名规范说明
3. 如保留旧 PG migration，则需要明确“仅历史参考、不再执行”的边界

#### 任务

1. 从当前最新 schema 反推 MySQL 基线
2. 为核心业务表生成 MySQL 建表语句
3. 为索引、唯一约束、外键、默认值做 MySQL 对齐
4. 识别无法直接平移的 PostgreSQL 特性并重新建模

#### 不建议做法

- 不建议逐条翻译 161 个历史 migration 后再串行执行

#### 验收

- 全新 MySQL 数据库可从零建出当前系统所需 schema

---

### 3.3 PostgreSQL 特性替代清单（迁移层）

#### 需要逐项处理的语法类型

- `BIGSERIAL`
- `TIMESTAMPTZ`
- `JSONB`
- `TEXT[]`
- `::type`
- `ON CONFLICT`
- `RETURNING`
- `CREATE OR REPLACE FUNCTION`
- `pg_indexes`
- `pg_constraint`
- `GIN` / `TRGM`
- `PARTITION`

#### 任务

对每一类语法给出 MySQL 替代方案，并在迁移说明中记录：

1. 原 PostgreSQL 用法
2. MySQL 替代实现
3. 是否存在行为差异
4. 是否需要业务代码补偿

#### 验收

- 形成明确的迁移语法映射表

---

### 3.4 Migration 测试与空库验证

#### 文件

- `backend/internal/repository/*migration*test*.go`
- 如需新增：MySQL migration smoke test

#### 任务

1. 增加空库 migration smoke test
2. 验证重复启动幂等
3. 验证 migration 记录表校验逻辑
4. 验证启动时自动 migration 可成功执行

#### 验收

- 从空库到可启动状态可自动完成
- 重复执行不报错

---

## 4. 阶段 1+2 具体执行顺序

建议按以下顺序落地：

1. `backend/go.mod` / `go.sum`
2. `backend/internal/config/config.go` + `config_test.go`
3. `backend/internal/repository/ent.go`
4. `backend/internal/setup/setup.go` + `cli.go` + `setup_test.go`
5. `backend/ent/schema/**/*.go`
6. 重新生成 `backend/ent/**`
7. `backend/internal/repository/migrations_runner.go`
8. `backend/migrations/` 新建 MySQL 基线
9. migration smoke test
10. `deploy/docker-compose*.yml` / `.env.example` / `config.example.yaml`
11. `deploy/README.md` / `deploy/DOCKER.md`

---

## 5. 每一步的最小验证建议

### 步骤 1-3 后

- `go test` 仅跑配置与初始化相关测试
- 确认后端可编译

### 步骤 4-6 后

- 本地连接 MySQL，验证 `InitEnt` 与 setup 链路

### 步骤 7-9 后

- 使用空 MySQL 库做 migration smoke test

### 步骤 10-11 后

- 用 docker compose 拉起最小环境
- 验证应用启动到健康检查通过

---

## 6. 建议先做的代码前确认

正式进入改代码前，建议锁定以下实施决策：

1. MySQL 目标版本：建议固定 8.0.x
2. 默认字符集：建议 `utf8mb4`
3. 默认排序规则：需统一决定，避免唯一键与大小写行为漂移
4. 时区策略：数据库统一 UTC，还是跟随应用配置
5. migration 策略：确认采用“新基线”而非“逐条翻译历史迁移”

---

## 7. 下一步

在本清单确认后，可按以下方式进入实现：

1. 先只实施“步骤 1-4”，把应用切到 MySQL 驱动并能连库
2. 再实施 migration 基线与 runner 改造
3. 应用启动成功后，再进入 Repository 业务 SQL 替换
