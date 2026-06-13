---
doc_type: issue-fix
issue: 2026-06-10-account-import-template-deleted-group
path: fast-track
fix_date: 2026-06-10
tags: [account-import, groups, soft-delete]
---

# 账号导入模板引用已删除分组修复记录

## 1. 问题描述

分组删除后，如果账号导入模板里仍保存着该分组 ID，套用模板导入账号时，旧分组 ID 仍会进入表单状态和提交 payload，导致账号创建后又显示已删除分组。

## 2. 根因

- 分组删除是软删除，正常分组列表只返回未删除分组。
- `frontend/src/components/admin/account/AccountDataImportForm.vue` 原来直接使用模板里的 `applyGroupIds`，没有按当前可用分组清洗。
- `backend/internal/service/admin_service.go` 的 `CreateAccount` 创建路径原来没有在写入前验证 `GroupIDs` 是否仍是未软删除分组，导入场景可能把历史模板中的旧 ID 写回 `account_groups`。

## 3. 修复方案

- 前端在套用模板、保存模板、构造导入 payload 时，统一按当前 `props.groups` 过滤 `applyGroupIds`，同时去重。
- 当前分组列表变化时，重新清洗已有选择，避免删除/刷新后残留旧 ID。
- 后端 `CreateAccount` 在账号创建和绑定前调用 `validateGroupIDsExist`，只接受存在且未软删除的分组。

## 4. 改动文件清单

- `frontend/src/components/admin/account/AccountDataImportForm.vue`
- `frontend/src/__tests__/integration/data-import.spec.ts`
- `backend/internal/service/admin_service.go`
- `backend/internal/service/admin_service_create_account_group_test.go`
- `backend/internal/service/account_service_tags_test.go`
- `backend/internal/handler/admin/account_data_handler_test.go`

## 5. 验证结果

- `npm run test:run -- src/__tests__/integration/data-import.spec.ts`
  - 13 个前端导入表单测试通过。
- `go test -tags=unit ./internal/service`
  - 后端 service unit 测试通过。
- `go test ./internal/handler/admin -run 'TestImportData_(ApplyGroupIDs_BindsToGroups|ApplyGroupIDs_ReportsDeletedGroup|NilApply_BehavesLikeBefore)'`
  - 导入 handler 相关冒烟测试通过。
- `git diff --check`
  - 通过；仅提示两个前端文件下次 Git 触碰时 CRLF 会转 LF。

## 6. 遗留事项

暂无。
