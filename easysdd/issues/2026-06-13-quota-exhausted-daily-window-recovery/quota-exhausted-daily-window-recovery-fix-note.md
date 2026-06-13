---
doc_type: issue-fix
issue: 2026-06-13-quota-exhausted-daily-window-recovery
path: fast-track
fix_date: 2026-06-13
tags:
  - subscription
  - quota_exhausted
  - daily-window
  - daily-overdraft
---

# quota_exhausted 日窗口过期后不恢复修复记录

## 1. 问题描述

订阅未过期，但在日额度透支场景中用满当前日窗口后被标记为 `quota_exhausted`。当滚动 24 小时日窗口已经过期后，后续请求仍无法重新选中该订阅；请求会绕到非订阅余额/其它组路径，`usage_logs.subscription_id` 变为 `NULL`。

典型数据特征：

- `status = quota_exhausted`
- `daily_window_start` 已超过 24 小时
- `daily_usage_usd` 接近当前日窗口最大可用额度
- `expires_at` 仍未过期
- 周期总透支池仍有剩余额度

## 2. 根因

`SubscriptionService.GetActiveSubscription()` 的热路径先调用 `UserSubscriptionRepository.GetActiveByUserIDAndGroupID()`。仓储实现只筛选 `status = active` 且未过期的订阅。

因此，已经变为 `quota_exhausted` 的订阅不会进入后续 `ValidateAndCheckLimits()` / `CheckAndResetWindows()`，也就没有机会在日窗口过期后清零 `daily_usage_usd` 并恢复为可用状态。

## 3. 修复方案

保持仓储层 `GetActiveByUserIDAndGroupID()` 的 active 语义不变，只在 `SubscriptionService.GetActiveSubscription()` 的 active 查询返回 `ErrSubscriptionNotFound` 后增加窄恢复分支：

1. 宽松回查同用户、同分组订阅。
2. 仅当订阅状态为 `quota_exhausted`、未过期、已开始时继续。
3. 执行 `CheckAndResetWindows()`，让过期日窗口落库重置。
4. 重新按当前 group 限额检查 daily / weekly / monthly 是否仍超限。
5. 只有窗口重置后确实不再超限，才把状态更新回 `active`，并失效 L1 与 billing cache。

如果总透支池/周期上限仍然耗尽，则保持 `quota_exhausted` 不恢复。

## 4. 改动文件清单

- `backend/internal/service/subscription_service.go`
  - `GetActiveSubscription()` 在 active 查询未命中时尝试恢复可恢复的 `quota_exhausted` 订阅。
  - 新增 `reactivateQuotaExhaustedSubscriptionIfRecoverable()`，封装恢复前置校验、窗口重置、限额复查、状态恢复和缓存失效。

- `backend/internal/service/subscription_reactivation_test.go`
  - 新增回归测试：日窗口过期且总池未耗尽时，`quota_exhausted` 订阅可恢复为 `active`。
  - 新增保护测试：总池已耗尽时，订阅仍保持 `quota_exhausted`。

## 5. 验证结果

已执行：

```bash
go test ./internal/service -run "TestGetActiveSubscription_QuotaExhaustedDailyOverdraft|TestAssignOrExtendSubscription_QuotaExhaustedRenewal" -count=1 -v -timeout 180s
go test ./internal/service -count=1 -timeout 300s
go test ./internal/server/middleware -count=1 -timeout 180s
```

结果均通过。

## 6. 遗留事项

- 本次只修复后端状态恢复逻辑，没有改前端展示文案。页面上的“透支剩余额度”仍表示周期总池剩余；如果希望避免误解，后续可以单独调整展示，把“当前日窗口剩余”和“周期总池剩余”分开显示。
- 当前修复触发点是下一次请求进入订阅选择时恢复，不是定时任务主动扫表恢复。
