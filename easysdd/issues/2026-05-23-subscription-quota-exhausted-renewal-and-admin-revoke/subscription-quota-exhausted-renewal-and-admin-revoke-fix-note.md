---
doc_type: issue-fix
issue: 2026-05-23-subscription-quota-exhausted-renewal-and-admin-revoke
status: confirmed
severity: P1
summary: 修复 quota_exhausted 订阅续费后仍不可用，以及后台订阅管理无法撤销 quota_exhausted 订阅的问题
tags:
  - subscription
  - quota_exhausted
  - renewal
  - admin
---

# Subscription quota exhausted renewal and admin revoke Fix Note

## 修复结果

- 修复了用户订阅进入 `quota_exhausted` 后，支付续费成功但仍然无法继续使用的问题。
- 修复了后台订阅管理页面中，`quota_exhausted` 状态订阅不显示“撤销”按钮的问题。

## 根因

1. `backend/internal/service/subscription_service.go`
   - `AssignOrExtendSubscription` 在 `quota_exhausted` 场景下走的是“普通续期”路径，只延长 `expires_at`，没有重开周期、没有清空窗口用量。
   - 结果是续费后虽然时间延长，但旧的 weekly/monthly usage 仍然超限，后续校验会继续拒绝使用。

2. `frontend/src/views/admin/SubscriptionsView.vue`
   - 管理页的撤销按钮仅对 `active` 状态显示，`quota_exhausted` 被错误排除。
   - 后端撤销接口本身可用，问题仅在前端操作入口缺失。

## 修改内容

- `backend/internal/service/subscription_service.go`
  - 将 `quota_exhausted` 续费视为需要“重开新周期”的场景。
  - 续费时重置 `starts_at` / usage windows / daily-weekly-monthly usage，并恢复 `active`。

- `backend/internal/service/subscription_reactivation_test.go`
  - 新增回归测试：
    - `TestAssignOrExtendSubscription_QuotaExhaustedRenewalRestartsPeriodAndRestoresUsability`

- `frontend/src/views/admin/SubscriptionsView.vue`
  - 允许 `quota_exhausted` 状态显示“撤销”按钮。

- `frontend/src/api/admin/subscriptions.ts`
  - 补齐前端筛选类型中的 `quota_exhausted`。

## 验证

- 前端类型检查通过：
  - `frontend`: `npm run typecheck`

- 后端测试通过：
  - `go test ./internal/service -run "TestAssignOrExtendSubscription_"`
  - `go test ./internal/service -run "TestValidateSubOrder(ExplicitExtend|Restart)RequiresActiveSubscription"`
  - `go test ./internal/service -run "Test(DoSubPersistsSubscriptionIDToOrder|AssignOrExtendSubscription_)"`

## 备注

- 本次修复聚焦支付续费链路与后台订阅管理操作入口。
- 未顺手改动其他不在本次范围内的订阅调整语义。
