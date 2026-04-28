# 模型广场验收报告

> 阶段：阶段 3（验收闭环）
> 验收日期：2026-04-26
> 关联方案 doc：`easysdd/features/2026-04-26-model-marketplace/model-marketplace-design.md`

## 1. 接口契约核对

对照方案 doc 第 2 节接口契约，核查结果如下。

- [x] 用户数据源：`GET /api/v1/channels/available`
  - 代码落点：`frontend/src/api/channels.ts`
  - 页面使用：`frontend/src/views/user/ModelMarketplaceView.vue`
  - 结果：一致

- [x] 管理员数据源：`GET /api/v1/admin/channels` + `GET /api/v1/admin/groups/all`
  - 代码落点：
    - `frontend/src/api/admin/channels.ts`
    - `frontend/src/api/admin/groups.ts`
  - 页面使用：`frontend/src/views/user/ModelMarketplaceView.vue`
  - 结果：一致

- [x] 前端归一结构 `MarketplaceProviderFacet / MarketplaceChannelInstance / MarketplaceModelItem`
  - 代码落点：`frontend/src/utils/modelMarketplace.ts`
  - 结果：一致

- [x] 路由契约 `/model-marketplace`
  - 代码落点：`frontend/src/router/index.ts`
  - 结果：一致

- [x] Header-only 布局契约
  - 代码落点：
    - `frontend/src/components/layout/HeaderOnlyLayout.vue`
    - `frontend/src/views/user/ModelMarketplaceView.vue`
    - `frontend/src/router/meta.d.ts`
  - 结果：已与 design 对齐。当前实现采用“route meta 声明 + view 直接组合布局”双保险方式。

## 2. 行为与决策核对

对照方案 doc 第 1 节决策与约束，核查结果如下。

### 2.1 需求摘要逐项验证

- [x] 可按供应商快速缩小范围
  - 证据：`MarketplaceProviderRail.vue` + `filterMarketplaceModelItems`

- [x] 可明确看到同模型来自哪些渠道
  - 证据：`MarketplaceModelCard.vue` 内嵌 `MarketplaceChannelList.vue`

- [x] 可比较同模型不同渠道的价格差异
  - 证据：渠道实例级别保留 `pricing`

- [x] 大量模型场景支持分页
  - 证据：`ModelMarketplaceView.vue` 使用 `Pagination`

### 2.2 明确不做逐项核对

- [x] 不再把模型广场放在 sidebar
  - 证据：`AppSidebar.vue` 已移除入口

- [x] 不保留旧“表格视图 / 卡片视图切换”
  - 证据：旧双视图文案与组件已清理

- [x] 不新增模型广场专用后端接口
  - 证据：只复用现有 `channels/admin channels/admin groups` 接口

- [x] 不把模型广场做成渠道管理页
  - 证据：页面无 CRUD 动作，仅浏览/筛选/分页

### 2.3 关键决策落地

- [x] 决策 A：header 入口 + 独立探索页
  - 落点：`AppHeader.vue`、`HeaderOnlyLayout.vue`

- [x] 决策 B：左筛选 + 右紧凑结果区
  - 落点：`ModelMarketplaceView.vue`

- [x] 决策 C：模型主项 + 渠道实例列表
  - 落点：`modelMarketplace.ts`、`MarketplaceModelCard.vue`

- [x] 决策 D：左侧按供应商聚合，渠道在右侧结果区表达
  - 落点：`collectMarketplaceProviderFacets`、`MarketplaceProviderRail.vue`

- [x] 决策 E：管理员 / 普通用户走不同拉取路径
  - 落点：`ModelMarketplaceView.vue`

## 3. 测试约束核对

- [x] C1：sidebar 中不再有模型广场菜单项
  - 验证方式：代码审阅 + grep
  - 结果：通过

- [x] C2：header 中存在模型广场入口
  - 验证方式：代码审阅 + grep
  - 结果：通过

- [x] C3：进入页面后不渲染 `AppSidebar`
  - 验证方式：代码审阅
  - 结果：通过

- [x] C4：供应商数量按模型主项数计算
  - 验证方式：单测
  - 结果：通过

- [x] C5：点击供应商后结果正确过滤
  - 验证方式：纯函数验证
  - 结果：通过

- [x] C6：同模型多渠道必须可区分
  - 验证方式：单测 + 组件结构审阅
  - 结果：通过

- [x] C7：分页 / 每页数量 / 搜索联动
  - 验证方式：代码审阅 + typecheck
  - 结果：通过

- [x] C8：旧 `/available-channels` 路由兼容
  - 验证方式：路由配置核对
  - 结果：通过

### 已执行验证

- [x] `pnpm --dir D:/data/sub2api/frontend typecheck`
- [x] `pnpm --dir D:/data/sub2api/frontend test:run -- src/utils/__tests__/modelMarketplace.spec.ts`

### 浏览器核对

- [x] 已确认本地前端可访问，`http://localhost:3000/` 正常打开
- [x] 已确认新路由存在，浏览器可命中 `/model-marketplace`
- [ ] 真实登录态下的完整 UI 停留验收
  - 阻塞原因：当前本地实例存在既有鉴权刷新链问题，Playwright 会被现网实例上的 `401 -> refresh -> redirect` 干扰，无法稳定停留在目标页完成整套 mock 验证
  - 处理结论：本轮以前端代码、路由、typecheck、归一层测试为主完成验收；若要补最终肉眼验收，建议在你本地真实登录态下再点一次 header 入口做最终确认

## 4. 术语一致性

对照方案 doc 第 0 节术语约定，结果如下。

- 模型广场
  - 命中位置：`AppHeader.vue`、`router/index.ts`、`ModelMarketplaceView.vue`、i18n
  - 结果：一致

- Header-only 布局
  - 命中位置：`HeaderOnlyLayout.vue`、`router/meta.d.ts`、design/architecture
  - 结果：一致

- 模型主项
  - 代码承载结构：`MarketplaceModelItem`
  - 结果：一致

- 渠道实例
  - 代码承载结构：`ModelMarketplaceChannelEntry`
  - 结果：一致

未发现新的冲突术语。

## 5. 架构归并

- [x] 新增架构文档：`easysdd/architecture/frontend-model-marketplace.md`
  - 已归并内容：
    - header 入口位置
    - Header-only 布局能力
    - 模型广场的数据归一层边界
    - provider facet / 模型主项 / 渠道实例 三层结构

- [x] 与现有项目级文档关系
  - 当前仓库此前没有完善的 `easysdd/architecture/` 目录
  - 本次已补最小可用架构文档，满足 feature 完成后的可追溯要求

- [x] `AGENTS.md` 是否需要补规约
  - 结论：本次不需要
  - 原因：没有新增跨 feature 的长期编码约束

## 6. 遗留

- 浏览器完整肉眼验收仍建议你在本地真实登录态补点一次：
  - header 是否在铃铛右侧展示“模型广场”
  - 点击后是否进入无 sidebar 的独立页面
  - 左侧供应商筛选、右侧模型卡片、分页是否符合预期

- 当前未纳入本轮的后续优化：
  - 若后续数据量进一步增大，可考虑给渠道实例列表加更明确的排序策略
  - 若后续需要更强筛选，可再加二级“按渠道筛选”，但不属于本轮范围
