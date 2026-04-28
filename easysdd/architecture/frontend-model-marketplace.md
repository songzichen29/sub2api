# Frontend 模型广场架构说明

## 1. 目标

模型广场是一个从全局 header 进入的独立探索页，用于展示：

- 用户或管理员当前可见的模型集合
- 同一模型下的多个渠道实例
- 各渠道实例所属分组与定价摘要

它不是 CRUD 管理页，也不是 sidebar 常规菜单页。

## 2. 页面入口与布局

### 2.1 入口

- 入口组件：`frontend/src/components/layout/AppHeader.vue`
- 入口位置：`AnnouncementBell` 右侧
- 开关：`FeatureFlags.availableChannels`

### 2.2 布局

- 页面容器：`frontend/src/components/layout/HeaderOnlyLayout.vue`
- 行为：保留 `AppHeader`，不渲染 `AppSidebar`
- 路由声明：`frontend/src/router/index.ts` 中 `/model-marketplace`
- 路由 hint：`meta.layout = 'header-only'`

当前仓库尚未建立统一的 route-meta layout dispatcher，因此页面本身直接组合 `HeaderOnlyLayout` 作为稳定落点。

## 3. 数据来源与角色差异

模型广场不新增专用后端接口，继续复用现有数据源：

### 3.1 普通用户

- `GET /api/v1/channels/available`
- 文件：`frontend/src/api/channels.ts`

### 3.2 管理员

- `GET /api/v1/admin/channels`
- `GET /api/v1/admin/groups/all`
- 文件：
  - `frontend/src/api/admin/channels.ts`
  - `frontend/src/api/admin/groups.ts`

## 4. 前端归一层边界

归一层文件：`frontend/src/utils/modelMarketplace.ts`

### 4.1 核心结构

- `MarketplaceProviderFacet`
  - 左侧供应商筛选栏的聚合项
- `MarketplaceModelItem`
  - 右侧结果区的一条模型主项
- `ModelMarketplaceChannelEntry`
  - 模型主项下的渠道实例

### 4.2 核心函数

- `buildMarketplaceModelItems`
  - 以 `platform + model_name` 聚合模型主项
- `collectMarketplaceProviderFacets`
  - 从模型主项集合提取供应商 facet
- `filterMarketplaceModelItems`
  - 统一处理搜索词 + 供应商筛选
- `transformAdminChannelsToAvailableChannels`
  - 把管理员渠道数据转成用户侧可消费形状

### 4.3 约束

- 同模型下多个渠道实例不可丢失
- 搜索必须覆盖：
  - 模型名
  - 供应商
  - 渠道名
  - 渠道描述
  - 分组名

## 5. 页面分层

### 5.1 View

- `frontend/src/views/user/ModelMarketplaceView.vue`

负责：

- 拉取数据
- 区分管理员 / 普通用户数据源
- 管理搜索、筛选、分页状态
- 组合页面布局

### 5.2 左侧筛选栏

- `frontend/src/components/channels/MarketplaceProviderRail.vue`

负责：

- 显示供应商 facet
- 展示每个供应商对应模型数
- 单选供应商与重置

### 5.3 模型主项卡片

- `frontend/src/components/channels/MarketplaceModelCard.vue`

负责：

- 展示模型名
- 展示供应商标签
- 展示该模型下渠道数
- 承载渠道实例列表

### 5.4 渠道实例列表

- `frontend/src/components/channels/MarketplaceChannelList.vue`

负责：

- 展示渠道名与描述
- 展示分组 badge
- 展示定价摘要
- 默认只展示前 2 条，其余折叠展开

## 6. 兼容策略

- 兼容路由：`/available-channels` → `/model-marketplace`
- 旧 sidebar 入口已移除
- 旧双视图（表格/卡片）已下线，不再作为长期 UI 形态保留
