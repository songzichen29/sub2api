---
doc_type: feature-design
feature: 2026-05-09-group-free-usage-buckets
status: approved
summary: 将用户余额拆分为免费/付费额度桶，并允许管理员为分组配置可消耗的免费额度金额或次数；返利转入按付费额度处理
tags: [group, balance, billing, quota, payment, affiliate, migration]
---

# 分组免费额度 / 付费额度分桶 design

## 0. 术语约定

| 术语 | 定义 | 防冲突结论 |
|---|---|---|
| 免费额度桶 / `free_balance` | 用户可消费但不允许触发付费权益的余额桶。来源包括默认赠送、注册来源赠送、首次绑定赠送、赠送型兑换码、管理员赠送。 | 已 grep `free_balance` / `paid_balance` / `legacy_balance`，当前仓库没有同名持久字段；现有 `users.balance` 是单一总余额，新增字段命名可直接使用。 |
| 付费额度桶 / `paid_balance` | 用户可消费且视为付费获得的余额桶。来源包括支付充值和返利转入。 | 已 grep 支付与返利链路：支付充值最终走 `RedeemTypeBalance -> userRepo.UpdateBalance`，返利转入走 `affiliate_repo.AddBalance + AddTotalRecharged`；当前没有独立付费桶。 |
| 历史余额桶 / `legacy_balance` | 历史系统中无法精确回溯来源的存量余额，迁移时暂存于该字段，避免错误把不确定余额判成免费或付费。 | 当前仓库无同名字段；保留该字段是为了让首轮迁移只“锁定能确定的付费部分”，其余留待后续清理。 |
| 分组免费额度金额上限 / `free_usage_limit_usd` | 配在 `groups` 表上的 USD 金额。命中该分组时，用户最多可在该分组消耗这么多免费额度金额；超过后只允许继续消耗 `paid_balance/legacy_balance`。`NULL` 表示不限制。 | 已 grep `daily_limit_usd` / `weekly_limit_usd` / `monthly_limit_usd`，这些字段只服务订阅分组；本字段服务标准计费分组的“免费额度可见边界”，不能复用订阅限额字段。 |
| 分组免费请求次数上限 / `free_usage_limit_requests` | 配在 `groups` 表上的免费请求次数。命中该分组并使用免费额度时，累计次数达到上限后，该分组不再允许消耗免费额度。`NULL` 表示不限制。 | 当前仓库无按 group 维度的用户请求计数表；需要新增持久化或缓存回写机制。 |
| 免费消耗快照 / `GroupFreeUsage` | 新增的 `(user_id, group_id)` 聚合使用记录，至少记录免费消耗金额与免费请求次数，用于判断分组内免费额度是否耗尽。 | 当前只有订阅 usage window 与总 usage_logs，没有“标准分组免费额度聚合表”，因此需要新建独立表。 |

防冲突 grep 记录：

- `rg -n "free_balance|paid_balance|legacy_balance" backend/internal frontend/src`：无现成余额分桶字段或逻辑。
- `rg -n "default_balance|ResolveAuthSourceGrantSettings|ApplyProviderDefaultSettingsOnFirstBind" backend/internal/service frontend/src`：新用户默认余额、注册来源赠送、首次绑定赠送都直接加到 `users.balance`，没有来源分桶。
- `rg -n "AddBalance\(|AddTotalRecharged\(|affiliate_balance|admin_balance|RedeemTypeBalance" backend/internal/service backend/internal/repository`：支付充值、返利转入、管理员加余额、兑换码赠送都落在单一余额字段；`total_recharged` 还会被返利转入增加，不能作为“纯付费余额”。
- `rg -n "subscription_type|daily_limit_usd|GroupsView.vue" backend/internal frontend/src`：分组已具备 `standard/subscription` 维度与订阅限额 UI，可复用同一管理页追加免费额度设置。

## 1. 决策与约束

### 1.1 需求摘要

**做什么**：

1. 把现有单一 `users.balance` 演进为“免费额度桶 + 付费额度桶 + 历史余额桶”三段式结构。
2. 以后所有新入账按来源进入明确桶：支付充值和返利转入进入 `paid_balance`；默认赠送/活动赠送/赠送兑换码/管理员赠送进入 `free_balance`。
3. 管理员可在分组中配置“该分组允许消耗的免费额度金额（USD）”和/或“该分组允许消耗的免费请求次数”；首版不做模型级免费策略。
4. 用户请求命中标准计费分组时，系统先判断该分组是否还允许消耗免费额度；若允许，则优先消耗 `free_balance`，不足部分允许继续拆分消耗 `paid_balance`，最后兜底消耗 `legacy_balance`；若该分组免费资格不可用，则直接跳过免费桶。
5. 首轮迁移不尝试精确回算历史免费余额：只把“能确定是付费获得”的存量余额迁到 `paid_balance`，其余存量迁到 `legacy_balance`，`free_balance` 从功能上线后新增赠送开始积累。

**为谁**：

- admin：在分组中定义“免费额度能否用在这个分组，以及能用多少金额/次数”。
- user：获得清晰的免费/付费扣费顺序，免费额度先在允许的分组里消耗。
- 系统/财务：把支付充值、返利转入、赠送额度区分开，避免后续再用推测公式回算。

**成功标准**：

1. `users` 表新增 `free_balance`、`paid_balance`、`legacy_balance`，旧 `balance` 仍保留为兼容读模型或由代码聚合成 `free + paid + legacy`。
2. 支付充值履约进入 `paid_balance`；返利转入进入 `paid_balance`；默认赠送/来源赠送/首次绑定赠送/赠送型兑换码/管理员赠送进入 `free_balance`。
3. 分组表新增 `free_usage_limit_usd`、`free_usage_limit_requests`；后台分组管理支持保存与回显。
4. 运行时对标准计费分组执行免费资格判断：
   - 先看用户 `free_balance > 0`
   - 再看该分组金额/次数阈值是否未耗尽
   - 满足则本次请求记作“免费消耗”，否则记作“付费消耗”。
5. 真实扣费顺序固定为 `free_balance -> paid_balance -> legacy_balance`；订阅分组仍走订阅配额逻辑，不参与余额三桶扣费。
6. 历史迁移脚本可以把“已完成余额充值订单 + 返利转入”识别出的确定付费额度迁入 `paid_balance`，剩余存量迁入 `legacy_balance`，不因猜错历史赠送值而损坏现网余额。
7. 前后端能展示至少基础余额拆分（用户总余额、admin 用户详情/余额历史可见免费/付费/历史三桶数值），且退款链路只扣付费相关桶。

**明确不做**：

- 不在本 feature 里清洗每个历史用户到底“起始送过 10 还是 5”；这类历史运营规则不作为首次迁移精确回算依据。
- 不在本 feature 里把 `legacy_balance` 强制归类为免费或付费；先保留不确定余额，后续再决定是否清零、转免费或单独促迁。
- 不改变订阅分组 `subscription_type=subscription` 的配额逻辑；新分组免费额度功能只作用于 `subscription_type=standard`。
- 不引入“按模型维度”的免费额度限制；首版只按分组维度控制免费策略，模型仍只负责产生最终成本。
- 不在本 feature 中新增复杂的运营活动系统；只改入账分桶与分组消耗规则。
- 不让普通用户在 UI 中手动选择“本次请求用免费还是付费”；完全由后端规则自动决定。
- 不在第一次上线时开放“管理员手动回填历史 free/paid”批量工具；如需补录，后续单独做 admin 数据修复工具。

### 1.2 关键决策

| 决策 | 选项 | 拍板 | 被拒理由 |
|---|---|---|---|
| 历史存量处理 | (a) `paid_balance + legacy_balance` 双阶段迁移 (b) 直接按公式回算 `free_balance = balance - recharge` (c) 全量当免费 | (a) | (b) 现有 `total_recharged` 混入返利与管理员正向加额，且历史赠送值变化；(c) 会把用户真实付费权益误判成免费。 |
| 返利转入口径 | (a) 返利算付费额度 (b) 返利算免费额度 | (a) | 用户已明确拍板“返利获得的额度算付费的”；业务上它来自他人真实支付行为，且现有代码会累加 `total_recharged`。 |
| 运行时扣费顺序 | (a) `free -> paid -> legacy` 并允许拆分扣费 (b) `paid -> free -> legacy` (c) 允许前端选择 | (a) | 需求目标是“免费额度能在指定分组先消耗”；允许拆分扣费可避免零散免费余额长期滞留；(b) 会导致免费控制失去意义；(c) 会让 API 行为不可预测且前端难解释。 |
| 免费策略挂载位置 | (a) 分组级 (b) 模型级 (c) 分组+模型双层 | (a) | 现有系统的计费语义主要挂在 group（subscription_type / rate_multiplier / limit）；模型还存在 token / 按次 / 图片等多种成本形态，首版做到模型级会把免费策略与定价计算强耦合，范围过细。 |
| 分组控制维度 | (a) 同时支持金额和次数 (b) 只支持金额 (c) 只支持次数 | (a) | 用户需求说“额度或者次数”；两者都支持，但允许任一为空。 |
| 免费使用记录载体 | (a) 新增 `user_group_free_usage` 聚合表 (b) 每次从 `usage_logs` 聚合 (c) 只用 Redis | (a) | (b) 热路径太重；(c) 重启/回写一致性差且验收难。聚合表可与 usage 记录一起维护。 |
| 旧 `users.balance` 兼容方式 | (a) 保留为派生读值，写路径全部切三桶 (b) 立即删除旧字段 (c) 继续双写旧字段+三桶 | (c) 迁移期，最终收敛到 (a) | 现有大量代码直接读 `user.Balance`，一次性移除风险太高；迁移期双写旧字段，待全链路切换后再降级为派生读值。 |
| 管理员手动加余额默认桶 | (a) 默认免费，可选切付费 (b) 默认付费 (c) 沿用不区分 | (a) | 大多数管理员加余额是补偿/赠送；但后端接口应预留 `bucket` 选项，避免以后无法加付费余额。 |

### 1.3 假设，review 时可以反驳

- 假设 1：首版迁移完成后，所有历史用户的 `free_balance` 初始值统一为 `0`；只有上线后的新赠送才进入免费桶。
- 假设 2：`legacy_balance` 在资格判断上视为“付费兜底”，即分组免费限制耗尽后仍可继续消耗 `legacy_balance`。
- 假设 3：退款只针对余额充值订单，扣回时优先从 `paid_balance` 扣；若不足，再从 `legacy_balance` 扣；不从 `free_balance` 回收。
- 假设 4：管理员余额调整接口愿意新增一个可选 `bucket` 字段；若不传则保持“免费赠送”默认行为。
- 假设 5：免费请求次数按“每次成功计费请求 +1”累计，不区分 token 数，不因实际扣费为 0 而跳过（例如免费模型但占一次调用）。

### 1.4 主流程概述

```mermaid
sequenceDiagram
    participant Admin as 管理员
    participant User as 用户请求
    participant API as 网关/计费后端
    participant DB as 数据库
    participant Pay as 支付/返利来源

    Admin->>DB: 配置 groups.free_usage_limit_usd / free_usage_limit_requests
    Pay->>API: 余额充值完成 / 返利转入
    API->>DB: paid_balance += amount; balance += amount
    API->>DB: 赠送型入账 -> free_balance += amount; balance += amount

    User->>API: 发起标准计费分组请求
    API->>DB: 读取 user.free/paid/legacy + user_group_free_usage
    API->>API: 判断该分组免费金额/次数是否还可用
    alt 可用免费额度
        API->>DB: free_balance -= cost; user_group_free_usage.free_cost += cost; free_requests += 1
    else 免费额度不可用/已耗尽
        API->>DB: paid_balance 或 legacy_balance 扣费
    end
    API-->>User: 返回上游响应
```

## 2. 接口契约

### 2.1 数据库 / schema

```go
// 来源：backend/ent/schema/user.go
field.Float("free_balance").
    SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
    Default(0)
field.Float("paid_balance").
    SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
    Default(0)
field.Float("legacy_balance").
    SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
    Default(0)

// balance 迁移期继续保留；写路径要求与三桶和保持一致：
// balance = free_balance + paid_balance + legacy_balance

// 来源：backend/ent/schema/group.go
field.Float("free_usage_limit_usd").
    Optional().
    Nillable().
    SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
    Comment("该分组允许消耗的免费额度金额上限（USD）；NULL 表示不限制")
field.Int("free_usage_limit_requests").
    Optional().
    Nillable().
    Comment("该分组允许消耗的免费请求次数上限；NULL 表示不限制")

// 新表：user_group_free_usages
field.Int64("user_id")
field.Int64("group_id")
field.Float("free_usage_usd").
    SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
    Default(0)
field.Int("free_request_count").
    Default(0)
field.Time("created_at")...
field.Time("updated_at")...
index.Fields("user_id", "group_id").Unique()
```

迁移：

- PostgreSQL：新增一份用户余额分桶 + 分组免费额度 + 聚合表迁移。
- MySQL：新增对应幂等迁移。
- 首次迁移脚本额外做数据回填：
  1. 识别历史已完成余额充值订单累计到账额。
  2. 识别历史返利转入累计金额。
  3. `paid_balance = min(balance, balance_recharge_total + affiliate_transfer_total)`。
  4. `legacy_balance = balance - paid_balance`。
  5. `free_balance = 0`。

### 2.2 用户 / admin DTO

```jsonc
// 来源：backend/internal/handler/dto/types.go + frontend/src/types/index.ts
{
  "balance": 27.5,
  "free_balance": 5,
  "paid_balance": 12.5,
  "legacy_balance": 10
}
```

admin 用户余额历史/详情需追加：

```jsonc
{
  "bucket": "free", // free | paid | legacy（仅对新流水类型或调整记录）
  "balance_snapshot": {
    "free_balance": 5,
    "paid_balance": 12.5,
    "legacy_balance": 10
  }
}
```

首版如果历史流水做不到补 `bucket`，允许老记录 `bucket = null`。

### 2.3 分组 admin API

```jsonc
PUT /api/v1/admin/groups/7
{
  "subscription_type": "standard",
  "free_usage_limit_usd": 5,
  "free_usage_limit_requests": 100
}

// 200
{
  "id": 7,
  "subscription_type": "standard",
  "free_usage_limit_usd": 5,
  "free_usage_limit_requests": 100
}
```

约束：

```jsonc
// subscription_type=subscription 时不允许配置免费额度，服务端归一为 null
PUT /api/v1/admin/groups/9
{
  "subscription_type": "subscription",
  "free_usage_limit_usd": 5,
  "free_usage_limit_requests": 100
}

// 200
{
  "subscription_type": "subscription",
  "free_usage_limit_usd": null,
  "free_usage_limit_requests": null
}
```

### 2.4 管理员余额调整 API

```jsonc
POST /api/v1/admin/users/42/balance
{
  "balance": 5,
  "operation": "add",
  "notes": "活动赠送",
  "bucket": "free"
}
```

兼容规则：
- 未传 `bucket` → 默认 `free`
- `operation=subtract` 且指定 `bucket` 时，只从该桶扣；若不指定，则按 `legacy -> paid -> free` 或另定规则（见 1.3 假设，可在 review 中拍板）

### 2.5 运行时计费资格与扣费契约

新增内部判定对象：

```go
// 来源：新增 backend/internal/service/balance_bucket.go
// DecideBalanceBucket 返回本次请求使用哪个余额桶。
type BalanceBucketDecision struct {
    Bucket string // free | paid | legacy
    UseFree bool
    Reason string // debug/audit only
}
```

判定逻辑（标准分组，免费策略只挂在 group 上；模型仍只参与成本计算）：

```go
if group.SubscriptionType == "subscription" {
    // 仍走订阅配额
}
if user.FreeBalance > 0 && groupAllowsFreeUsage(userID, groupID, costUSD) {
    return free
}
if user.PaidBalance > 0 {
    return paid
}
return legacy
// 若允许拆分扣费，最终执行阶段会生成多桶扣费计划，而不是要求单桶足额
```

### 2.6 返利转入与支付履约契约

```jsonc
// 返利转入完成后，admin/user overview 中应可观察到：
{
  "balance": 30,
  "free_balance": 5,
  "paid_balance": 25,
  "legacy_balance": 0
}
```

要求：
- `affiliate_repo.TransferQuotaToBalance` 不再写单一 `AddBalance + AddTotalRecharged`，而是写 `AddPaidBalance + AddBalance + AddTotalRecharged`。
- `payment_fulfillment.go` 的余额充值履约同样写入 `paid_balance`。

## 3. 实现提示

### 3.1 目标文件状况评估

- `backend/internal/service/gateway_service.go`、`billing_cache_service.go`、`user_repo.go`、`payment_fulfillment.go`、`payment_refund.go` 是高热路径/高风险文件；新增分桶逻辑必须抽 helper，避免把条件散落在每个调用点。
- `frontend/src/views/admin/GroupsView.vue` 已接近超大文件；本次只在既有“订阅设置/计费设置”区域追加两个免费额度字段，不借机重构整页。
- `backend/internal/service/group_service.go` 这个旧服务结构相对老，但实际 admin 主要走 `admin_service.go + handler/admin/group_handler.go`；新增字段应优先沿当前主线贯通，避免双套实现偏移。
- 退款逻辑目前只支持余额订单，并直接看 `user.Balance` 计算可扣金额；若不一起改，会出现退款扣到了本不该回收的免费额度，因此退款链路必须纳入本 feature。

### 3.2 改动计划

| 文件 | 动作 | 说明 |
|---|---|---|
| `backend/ent/schema/user.go` | 追加到已有文件 | 新增 `free_balance` / `paid_balance` / `legacy_balance`。 |
| `backend/ent/schema/group.go` | 追加到已有文件 | 新增 `free_usage_limit_usd` / `free_usage_limit_requests`。 |
| `backend/ent/schema/user_group_free_usage.go` | 新建文件 | 新增用户-分组免费消耗聚合表。 |
| `backend/migrations/*.sql` / `backend/migrations/mysql/*.sql` | 新建文件 | schema 变更 + 历史回填 SQL/脚本。 |
| `backend/internal/service/user.go`、`dto/types.go`、`frontend/src/types/index.ts` | 追加字段 | user/group DTO 与 TS 类型贯通。 |
| `backend/internal/repository/user_repo.go` | 拆 helper + 追加 | 支持按桶加减余额、迁移期维护总 `balance` 一致。 |
| `backend/internal/repository/affiliate_repo.go` | 追加 | 返利转入写 `paid_balance`。 |
| `backend/internal/service/payment_fulfillment.go` | 追加 helper | 余额充值履约写 `paid_balance`；订阅购买不动余额分桶。 |
| `backend/internal/service/payment_refund.go` | 追加 helper | 退款扣回优先 `paid_balance`，再 `legacy_balance`。 |
| `backend/internal/service/auth_service.go` / `auth_oauth_first_bind.go` / 设置默认赠送相关代码 | 追加 | 默认赠送、来源赠送、首绑赠送改写 `free_balance`。 |
| `backend/internal/service/redeem_service.go` | 追加 | 赠送型兑换码写 `free_balance`；若后续管理员生成付费型兑换码再扩展类型。 |
| `backend/internal/service/balance_bucket.go` | 新建文件 | 封装免费资格判断、扣费顺序、桶选择。 |
| `backend/internal/service/user_group_free_usage.go` + repo | 新建文件 | 读写 `(user,group)` 免费消耗金额/次数。 |
| `backend/internal/service/billing_cache_service.go`、`gateway_service.go` | 追加 hook | 资格判断与实际扣费、缓存回写接入分桶决策。 |
| `backend/internal/handler/admin/group_handler.go`、`frontend/src/views/admin/GroupsView.vue` | 追加字段 | 后台分组设置界面支持免费金额/次数限制。 |
| `backend/internal/handler/admin/user_handler.go`、`frontend` 用户/admin 余额展示页面 | 追加字段 | 展示三桶余额，管理员调整支持 bucket。 |
| `frontend/src/i18n/locales/zh.ts`、`en.ts` | 追加词条 | 免费额度、付费额度、历史余额、分组免费额度限制等文案。 |

### 3.3 推进顺序

1. **schema 与类型贯通**：用户三桶余额、分组免费额度字段、免费使用聚合表、DTO/TS 类型先打通。退出信号：后端相关包编译通过，前端 typecheck 无缺字段。
2. **入账链路分桶**：默认赠送/首绑/支付充值/返利转入/管理员调整/兑换码分别写入正确桶，并保持 `balance = free + paid + legacy`。退出信号：单测能覆盖各来源落桶。
3. **历史迁移脚本**：实现 `paid_balance + legacy_balance` 首次回填，验证一组带支付/返利/赠送的 fixture 用户迁移后总余额不变。退出信号：迁移测试通过，样例数据总额守恒。
4. **免费资格判定与实际扣费**：新增运行时桶选择、按 group 判断免费金额/次数是否可用、真实扣费和聚合更新。退出信号：单测覆盖 free -> paid -> legacy 顺序、分组限制命中、订阅分组不走该逻辑。
5. **退款与管理端兼容**：退款只回收付费/历史桶；管理员调整支持 bucket。退出信号：退款测试和 admin balance 调整测试通过。
6. **前端配置与展示**：分组管理表单、用户/admin 余额展示、必要的说明文案落地。退出信号：界面能保存/回显配置，用户详情能看见三桶余额。
7. **回归与风险封口**：跑支付、返利、默认赠送、请求扣费最小回归集，并记录 `legacy_balance` 残留策略。退出信号：最小测试集通过或留下明确风险说明。

### 3.4 实现风险与约束

- `balance` 旧字段在迁移期不能立刻删；否则 `billing_cache_service`、refund、admin list、用户资料等大量旧代码会崩。
- 运行时资格判断和最终扣费必须使用同一套 decision/plan helper；否则会出现“预检查说有余额，真正扣费扣到另一个桶失败”的竞态。
- 免费请求次数统计需要明确定义“什么叫一次请求成功记账”；建议与 usage log 成功写入/实际产生 billing 事件绑定，而不是请求刚开始就+1。
- group 免费额度字段只对 `subscription_type=standard` 生效；订阅分组配置时前后端都应隐藏或清空。
- 历史迁移不能依赖 `total_recharged` 直接回推付费余额，因为它已被返利转入污染；必须以支付订单 + 返利转入台账为主。
- 退款如果继续看总 `balance` 直接扣，会把免费额度也回收掉；这是本 feature 必修链路，不可留到后面。
- 缓存层（auth cache / billing cache）若缓存的还是旧 `user.Balance`，上线后会出现 eligibility 与 DB 不一致；缓存结构也要同步扩字段或派生逻辑。

### 3.5 测试设计

| 功能点 | 验证方式 | 关键用例骨架 |
|---|---|---|
| 用户余额三桶 schema/DTO | repository/service/handler 单测 | user 创建后默认三桶=0；API 返回 `balance/free/paid/legacy`。 |
| 入账来源分桶 | service 单测 | 支付充值进 `paid_balance`；返利转入进 `paid_balance`；默认赠送/首绑/兑换码/管理员默认赠送进 `free_balance`。 |
| 历史迁移守恒 | migration/integration test | 构造有支付、返利、赠送的用户，迁移后 `free+paid+legacy == 原 balance`，且确定付费部分进入 `paid_balance`。 |
| 分组免费额度配置 | admin handler/front-end test | 标准分组可保存 `free_usage_limit_usd/requests`；订阅分组提交时字段被清空。 |
| 桶选择逻辑 | service unit test | `free_balance>0` 且 group 限额未耗尽 → free；金额耗尽/次数耗尽 → paid；paid=0 → legacy。 |
| 真实扣费 | gateway/service test | cost=1，free=0.5 paid=2 → 本次若要求单桶扣费需拒绝或拆分（需实现时拍板）；首版建议单桶足额才选该桶。free 足额时更新 `user_group_free_usage`。 |
| 退款回收 | payment_refund test | 退款 5 元，先扣 `paid_balance`；paid 不足再扣 `legacy_balance`；不动 `free_balance`。 |
| 前端展示 | vitest | 用户/admin 页面显示三桶余额；GroupsView 免费额度字段保存与回显。 |

实现时需要明确一个细节（目前作为测试用例驱动你在实现阶段再拍板也行）：
- 当 `free_balance` 不足以覆盖单次请求成本时，首版**允许拆分扣费**（先扣完 free，再扣 paid，再扣 legacy）。因此实现时上层对象应设计成“扣费计划（deduction plan）”，而不是“单桶决策”。

建议验证命令：

```bash
cd backend
go test ./internal/service ./internal/repository ./internal/handler/admin -run "Balance|Payment|Affiliate|Group|FreeUsage"
```

```bash
cd frontend
pnpm test -- --run src/views/admin/__tests__/GroupsView.spec.ts src/views/admin/__tests__/UsersView.spec.ts
pnpm typecheck
```

## 4. 与项目级架构文档的关系

- 现有仓库没有专门的“计费/余额架构”文档；本 feature 将把单余额模型升级成三桶余额模型，后续 acceptance 阶段应补一份 architecture 或 decision 文档记录“为什么保留 legacy_balance、为什么返利算 paid”。
- 关联现有 feature：
  - `2026-05-09-daily-limit-reset-payment`：同样扩展了 group/payment/billing 交叉区，本 feature 需要避免和其新增的 `daily_limit_reset_price` 语义冲突。
  - 邀请返利相关现有实现：`backend/internal/repository/affiliate_repo.go`、`backend/internal/service/affiliate_service.go`，本 feature 改口径“返利算 paid”后必须同步这些链路。
- 与用户默认赠送设置 (`default_balance`、`auth_source_default_*_balance`) 的关系：这些设置不废弃，只是落桶从“直接写总 balance”改为“写 free_balance 并同步总 balance”。

