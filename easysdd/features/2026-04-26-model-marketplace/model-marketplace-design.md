---
doc_type: feature-design
feature: 2026-04-26-model-marketplace
status: approved
summary: 将“模型广场”从侧边栏里的 available-channels 升级页，重构为从 header 进入的独立探索页：无应用侧边栏、左侧供应商筛选、右侧紧凑模型结果区，并显式区分同模型下的多个渠道实例。
tags: [frontend, model-marketplace, header-entry, marketplace, channels]
---

# 模型广场 Feature Design

## 0. 术语约定

- 模型广场：一个独立的模型探索页，从应用 header 进入，而不是应用左侧菜单项。
- 供应商筛选栏：模型广场左侧的固定筛选区，按平台 / 供应商维度展示可选项与数量，例如 OpenAI、Anthropic、Google。
- 模型主项：结果区中一条模型结果，代表同一 `platform + model_name` 的聚合展示。
- 渠道实例：同一模型主项下来自不同渠道的具体来源条目，必须保留渠道名、价格和分组，不允许被压平成“看不出来源”的单行摘要。
- Header-only 布局：保留全局顶栏 `AppHeader`，但不显示应用侧边栏 `AppSidebar` 的页面布局。

术语防冲突结果：已 grep `模型广场|marketplace|header-only|供应商筛选栏|渠道实例`，仓库内没有同名现成页面概念；当前最接近的是 `frontend/src/views/user/ModelMarketplaceView.vue` 的“用户侧可用渠道升级页”实现，以及 `frontend/src/components/layout/AppHeader.vue` 的顶部动作区。本次把“模型广场”重新定义为独立探索页，而不是 sidebar 内的常规业务页。

## 1. 决策与约束

### 1.1 用户目标

用户进入模型广场后，应能高效完成四件事：

1. 快速按供应商缩小范围，而不是在长表格或大留白卡片里逐页扫。
2. 明确看到某个模型来自哪些渠道，而不是只看到一个“聚合后”的名字。
3. 在不进入管理页的情况下快速比较同模型不同渠道的价格差异。
4. 在大量模型（数百条）情况下仍保持可浏览、可筛选、可翻页。

### 1.2 明确不做

- 不再把模型广场放在应用左侧菜单中；最终入口改为 `AppHeader` 里铃铛旁的独立按钮。
- 不延续当前“表格视图 / 卡片视图切换”方案；本次改成单一的探索页布局，避免双视图长期并存。
- 不把“多个渠道的同一模型”压平成不可区分的一条价格摘要；渠道实例必须可见。
- 不在第一阶段新增专用后端模型广场接口；先复用现有数据源完成前端重构。
- 不把模型广场做成渠道管理页；它是探索 / 浏览页，不承接 CRUD。

### 1.3 关键决策

#### 决策 A：模型广场从 sidebar 常规页面改为 header 入口的独立探索页

原因：

- 当前 sidebar 适合常规业务页，不适合“全屏探索型页面”。
- 用户明确要求入口放在铃铛旁边，点击后进入一个没有左侧菜单的新页面。
- 这类页面更像“全局能力入口”，而不是“账户页里的一个子菜单”。

代码锚点：

- 当前 header 动作区：`frontend/src/components/layout/AppHeader.vue`
- 当前带 sidebar 的布局：`frontend/src/components/layout/AppLayout.vue`

结论：

- 新入口放在 `AnnouncementBell` 旁边。
- 模型广场页面不再通过 `AppSidebar.vue` 暴露。
- 页面需要新的 Header-only 布局能力，而不是继续套 `AppLayout.vue`。

#### 决策 B：结果页采用“左侧供应商筛选栏 + 右侧紧凑模型结果区”，不保留当前表格/大卡片双方案

原因：

- 纯表格在字段多、模型多时横向膨胀严重，浏览成本高。
- 现有大留白卡片在模型多时密度过低，滚动过长。
- 用户给出的参考图更接近“左侧筛选 + 右侧浏览”的探索型信息架构。

结论：

- 左侧固定供应商筛选栏，展示所有可见供应商和数量。
- 右侧使用紧凑结果卡，不做当前那种大块空白卡片，也不回退到宽表格。
- 单卡内保留模型摘要 + 渠道实例列表，解决“同模型多渠道”不易区分的问题。

#### 决策 C：同模型多渠道的核心表达单位是“模型主项 + 渠道实例列表”

原因：

- 仅按模型聚合会丢失渠道来源，用户无法判断该价格对应哪条渠道。
- 仅按渠道摊平则会让同模型重复太多，难以比较。

结论：

- 仍按 `platform + model_name` 聚成模型主项。
- 但每个模型主项下必须显示渠道实例列表。
- 每个渠道实例至少展示：
  - 渠道名
  - 价格摘要
  - 分组 / 端点类型标签
  - 倍率信息（若有）

#### 决策 D：左侧筛选栏按供应商 / 平台聚合，具体渠道放在结果区内表达

原因：

- 真实渠道数量可能很多，直接把所有渠道都放左侧会导致筛选栏过长且噪音大。
- 用户给出的参考图左侧更接近“供应商聚合入口”。

结论：

- 左侧先按供应商 / 平台聚合（OpenAI、Anthropic、Google...）。
- 每个供应商展示模型数量。
- 具体渠道名不在左栏平铺，而是在模型主项内展示为渠道实例。
- 如果后续有明确需求，再单独加“按渠道筛选”的二级筛选，不在本轮一起做。

#### 决策 E：第一阶段继续复用现有数据源，管理员和普通用户按角色走不同拉取路径

原因：

- 已有代码里，普通用户和管理员可访问的数据语义本来就不同。
- 当前实现已证明：管理员若直接走 `/channels/available`，容易被“用户可见分组”语义影响。

结论：

- 普通用户：继续使用 `GET /api/v1/channels/available`
- 管理员：继续使用现有管理端渠道 / 分组接口并在前端归一
- 本轮不新增“模型广场专用 API”

代码锚点：

- 用户数据源：`frontend/src/api/channels.ts`
- 管理员数据源：`frontend/src/api/admin/channels.ts`、`frontend/src/api/admin/groups.ts`

### 1.4 放置位置

本 feature 仍属于“用户侧模型/渠道可见性展示”模块，但页面容器从普通 `AppLayout` 迁移到新的 Header-only 布局层。

也就是说，业务语义仍然是“模型广场”，但布局归属不再是“sidebar 页面”，而是“header 进入的独立探索页”。

### 1.5 被拒方案

- 方案 1：继续修当前表格/卡片双视图。
  - 拒绝原因：用户已经明确指出这两种形态都不对，继续修只是延长错误方向。
- 方案 2：左侧直接列所有具体渠道。
  - 拒绝原因：渠道规模增长后噪声过大，不如先按供应商聚合。
- 方案 3：把模型广场继续放在 sidebar，同时再在 header 里加一个重复入口。
  - 拒绝原因：会造成两套入口并存、页面身份模糊。

## 2. 接口契约

### 2.1 第一阶段继续复用的接口

普通用户：

- `GET /api/v1/channels/available`

管理员：

- `GET /api/v1/admin/channels`
- `GET /api/v1/admin/groups/all`

当前代码锚点：

- `frontend/src/api/channels.ts`
- `frontend/src/api/admin/channels.ts`
- `frontend/src/api/admin/groups.ts`
- `frontend/src/utils/modelMarketplace.ts`

### 2.2 前端归一后的页面数据模型

前端内部需要统一成更适合探索页的数据结构：

```ts
MarketplaceProviderFacet = {
  provider: string
  model_count: number
}

MarketplaceChannelInstance = {
  channel_name: string
  channel_description: string
  groups: UserAvailableGroup[]
  pricing: UserSupportedModelPricing | null
}

MarketplaceModelItem = {
  model_name: string
  provider: string
  channel_count: number
  channels: MarketplaceChannelInstance[]
}
```

归一规则：

1. 主键仍然是 `platform + model_name`。
2. 同模型下的每个渠道实例都要保留，不能只留最便宜一条。
3. 左侧供应商数量按 `MarketplaceModelItem` 聚合，不按渠道数聚合。
4. 搜索需命中：模型名、供应商、渠道名、渠道描述、分组名。

### 2.3 路由契约

主路由保留：

```ts
{
  path: '/model-marketplace',
  name: 'ModelMarketplace',
  component: () => import('@/views/user/ModelMarketplaceView.vue'),
  meta: {
    requiresAuth: true,
    requiresAdmin: false,
    titleKey: 'modelMarketplace.title',
    descriptionKey: 'modelMarketplace.description',
    layout: 'header-only'
  }
}
```

兼容路由保留：

- `/available-channels` → redirect 到 `/model-marketplace`

说明：

- 旧路径只保兼容，不再作为正式命名出现。
- 为了支撑“无左侧菜单”的页面形态，需要扩展 `RouteMeta` 或布局注入方式，让路由能声明 `header-only`。
- 当前实现选择“双保险”：
  - 路由 `meta.layout` 声明为 `header-only`
  - `ModelMarketplaceView.vue` 直接组合 `HeaderOnlyLayout`
  这样即使全局路由壳层暂未统一消费 `meta.layout`，页面行为仍然稳定落在 Header-only 布局上。

### 2.4 Header 入口契约

入口放置在 `frontend/src/components/layout/AppHeader.vue` 的右侧动作区：

- 位于 `AnnouncementBell` 旁边
- 文案：`模型广场`
- 点击进入 `/model-marketplace`

正式入口行为：

- 已登录用户可见
- 管理员与普通用户都可见
- 不依赖 sidebar featureFlag 暴露

## 3. 实现提示

### 3.1 改动计划

#### 步骤 1：先把模型广场从 sidebar 页面改为 header-only 页面

涉及文件：

- `frontend/src/components/layout/AppHeader.vue`
- `frontend/src/components/layout/AppLayout.vue`
- 新建 `frontend/src/components/layout/HeaderOnlyLayout.vue`
- `frontend/src/router/index.ts`
- `frontend/src/router/meta.d.ts`
- `frontend/src/components/layout/AppSidebar.vue`

实现要点：

- 在 header 加“模型广场”按钮
- 新建不含 sidebar 的布局组件
- 路由声明使用 `header-only`
- sidebar 中移除模型广场入口

退出信号：

- 铃铛旁能看到“模型广场”按钮
- 进入 `/model-marketplace` 时左侧应用菜单不显示
- sidebar 中不再出现模型广场入口

#### 步骤 2：重做模型广场的数据归一层，产出供应商 facet + 模型主项 + 渠道实例

涉及文件：

- `frontend/src/utils/modelMarketplace.ts`
- `frontend/src/views/user/ModelMarketplaceView.vue`
- 如必要，新建 `frontend/src/composables/useModelMarketplace.ts`

实现要点：

- 从现有数据源归一出供应商 facet
- 保留“同模型下多个渠道实例”
- 管理员和普通用户仍可共用同一结果页模型

退出信号：

- 左栏能显示供应商列表及数量
- 同模型存在多渠道时，页面上能明确区分各渠道实例
- 管理员和普通用户都能看到非空结果（前提是后台有数据）

#### 步骤 3：把结果区改成紧凑探索卡，而不是宽表格或大留白卡片

涉及文件：

- `frontend/src/views/user/ModelMarketplaceView.vue`
- 新建 `frontend/src/components/channels/MarketplaceProviderRail.vue`
- 新建 `frontend/src/components/channels/MarketplaceModelCard.vue`
- 新建 `frontend/src/components/channels/MarketplaceChannelList.vue`
- `frontend/src/components/channels/ModelMarketplacePricingSummary.vue`

实现要点：

- 左栏固定筛选，右侧结果区紧凑排布
- 卡片首屏只放关键信息：模型名、供应商、价格摘要、渠道数
- 渠道实例列表默认可见 1-2 条，其余通过展开查看
- 避免当前截图里的大面积空白

退出信号：

- 单屏可见模型数量明显高于当前大卡片方案
- 宽表格完全移除
- 同模型多渠道在卡片内可读

#### 步骤 4：补齐搜索、供应商筛选、分页/页大小，保证大量模型时可用

涉及文件：

- `frontend/src/views/user/ModelMarketplaceView.vue`
- 若已有分页组件可复用，则接入现有组件

实现要点：

- 顶部搜索框
- 左侧供应商单选 / 多选（二选一，本轮优先单选）
- 结果区分页
- 每页条数可调

退出信号：

- 500+ 模型情况下不会一次性展开成超长页面
- 切换供应商后结果正确收敛
- 搜索词与分页联动正常

#### 步骤 5：做兼容收尾，移除当前双视图心智

涉及文件：

- `frontend/src/views/user/AvailableChannelsView.vue`
- `frontend/src/components/layout/AppSidebar.vue`
- `frontend/src/i18n/locales/zh.ts`
- `frontend/src/i18n/locales/en.ts`

实现要点：

- 旧 available-channels 页面仅保留兼容 redirect/薄包装
- 移除“表格/卡片切换”的旧文案和状态
- 保持旧链接仍可访问，但不再作为主入口

退出信号：

- 用户只能从 header 正式进入模型广场
- 旧路由兼容可用
- 页面上不再出现“渠道视图 / 模型视图”双切换

### 3.2 高风险实现约束

- 不允许继续保留 sidebar 入口与 header 入口双入口并存。
- 不允许把同模型的多个渠道实例重新压平成一条不可追溯摘要。
- 不允许在首屏把所有渠道实例全部展开，导致 500+ 模型时页面长度爆炸。
- 不允许为了新页面样式直接把 `AppLayout.vue` 改坏其他页面；应新建 Header-only 布局。
- 不允许在没有设计确认前继续沿当前“表格 / 大卡片”方案追加细节。

### 3.3 测试设计

#### 功能点 1：入口从 sidebar 切到 header，且页面无左侧菜单

测试约束：

- sidebar 中不再有模型广场菜单项
- header 中铃铛旁出现模型广场入口
- 进入页面后不渲染 `AppSidebar`

验证方式：

- Header / Sidebar 组件测试
- 路由布局测试

关键用例骨架：

1. 登录用户访问任意业务页，header 显示“模型广场”按钮。
2. 点击按钮进入 `/model-marketplace`。
3. 进入后左侧应用菜单不存在。

#### 功能点 2：左侧供应商筛选栏正确聚合并筛选结果

测试约束：

- 供应商数量按模型主项数计算
- 点击供应商后右侧结果正确过滤
- 重置后恢复全部结果

验证方式：

- 归一函数测试 + 页面交互测试

关键用例骨架：

1. OpenAI 下有 20 个模型主项，左侧计数显示 20。
2. 点击 OpenAI 后只显示 OpenAI 模型。
3. 点击重置恢复全部供应商结果。

#### 功能点 3：同模型多渠道必须可区分

测试约束：

- 同一 `platform + model_name` 下有多个渠道时，页面必须能看到多个渠道实例
- 每个渠道实例的价格和分组归属可单独识别

验证方式：

- 纯函数聚合测试 + 结果卡 DOM 测试

关键用例骨架：

1. `gpt-5.4` 同时来自 channel-A / channel-B → 页面只出现一个模型主项，但内部有两条渠道实例。
2. 两个渠道实例价格不同 → 页面可直接区分。

#### 功能点 4：大量模型下仍可浏览

测试约束：

- 页面支持分页
- 每页数量可调
- 搜索与分页组合不出错

验证方式：

- 页面交互测试

关键用例骨架：

1. 总模型数 > 100 时，只渲染当前页结果。
2. 调整每页数量后，结果数量变化正确。
3. 搜索后分页重置到第一页。

#### 功能点 5：旧 available-channels 路由兼容但不再是主入口

测试约束：

- `/available-channels` 仍可访问
- 最终到达模型广场页面
- 不再从 sidebar 暴露

验证方式：

- 路由测试

关键用例骨架：

1. 访问 `/available-channels` → 到达 `/model-marketplace`
2. sidebar 中搜索不到模型广场入口

## 4. 与项目级架构文档的关系

当前仓库还没有完善的 `easysdd/architecture/` 文档体系；本 feature 暂以代码锚点记录架构位置：

- 全局顶栏：`frontend/src/components/layout/AppHeader.vue`
- 现有应用布局：`frontend/src/components/layout/AppLayout.vue`
- 用户侧路由：`frontend/src/router/index.ts`
- 现有模型广场页：`frontend/src/views/user/ModelMarketplaceView.vue`
- 用户数据源：`frontend/src/api/channels.ts`
- 管理员数据源：`frontend/src/api/admin/channels.ts`、`frontend/src/api/admin/groups.ts`

如果本轮实现完成并稳定，应补一份架构文档，明确：

1. header-only 页面布局能力
2. 模型广场的数据归一层边界
3. “供应商 facet / 模型主项 / 渠道实例”三层展示模型
