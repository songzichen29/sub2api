---
doc_type: issue-fix
issue: 2026-06-15-subscription-daily-limit-stale-cache
path: fast-track
fix_date: 2026-06-15
tags:
  - subscription
  - billing-cache
  - daily-limit
---

# 订阅每日限额缓存快照误判修复记录

## 1. 问题描述

用户真实订阅仍在有效期内，且当前日用量低于每日限额，但热路径在短时间内返回 `billing_error / 已超过每日使用限额`。

典型现场：

- user_id=110
- group_id=26
- subscription_id=127
- 日限额：80
- 当前真实日用量：66.9359596
- 16:26:22 到 16:27:00 连续被每日限额拦截，16:29 后同用户同分组恢复正常。

## 2. 根因

订阅鉴权和 handler 二次计费检查都可能直接信任热路径缓存快照：

- `SubscriptionService.ValidateAndCheckLimits` 使用 `GetActiveSubscription` 返回的 L1 订阅快照判断限额。
- `BillingCacheService.checkSubscriptionEligibility` 使用 Redis `billing:sub:<user_id>:<group_id>` 快照判断限额。

当缓存快照短暂显示 `daily_usage >= daily_limit`，代码会直接拒绝请求，没有在“即将拒绝用户”前回源 DB 做权威确认。

## 3. 修复方案

- 在 `ValidateAndCheckLimits` 判定日/周/月超限时，先从 DB 重新读取 active subscription 并复核一次。
  - DB 也超限：保持原拒绝行为。
  - DB 未超限：覆盖当前订阅对象、清理 L1/Redis 订阅缓存并放行。
- 在 `BillingCacheService.checkSubscriptionEligibility` 判定 Redis 订阅快照超限时，同样回源 DB 复核。
  - DB 未超限：删除并刷新 Redis 订阅缓存后放行。
  - DB 仍超限或回源失败：保持原拒绝行为。

## 4. 改动文件清单

- `backend/internal/service/subscription_service.go`
  - 增加超限前 DB 复核逻辑，避免 L1 订阅快照误杀请求。
- `backend/internal/service/billing_cache_service.go`
  - 增加 Redis 订阅缓存超限前 DB 复核与缓存刷新逻辑。
- `backend/internal/service/user_subscription_daily_quota_test.go`
  - 增加 L1 快照误判每日限额的回归测试。
- `backend/internal/service/billing_cache_service_test.go`
  - 增加 Redis 订阅缓存误判每日限额的回归测试。

## 5. 验证结果

已通过：

```bash
go test ./internal/service -run "TestValidateAndCheckLimits_RechecksStaleDailyLimitSnapshot|TestBillingCacheServiceCheckSubscriptionEligibility_RechecksStaleDailyLimitCache|TestBillingCacheServiceCheckSubscriptionEligibility|TestValidateAndCheckLimits_DailyCardDoesNotAllowSecondQuotaAfterMidnight" -count=1
go test -tags unit ./internal/server/middleware -run "TestSimpleModeBypassesQuotaCheck|TestAPIKeyAuthGatewayProtocolsReturnNativeBillingErrors" -count=1
```

结果：

- `ok github.com/Wei-Shaw/sub2api/internal/service`
- `ok github.com/Wei-Shaw/sub2api/internal/server/middleware`

## 6. 遗留事项

- 本次修复聚焦“缓存快照显示超限但 DB 权威数据未超限”的误拒绝场景。
- 若后续仍出现同类问题，应继续追查快照为什么会短暂高于 DB，例如异步缓存累加、请求幂等去重、DB 写入失败后的 Redis 更新顺序等。
