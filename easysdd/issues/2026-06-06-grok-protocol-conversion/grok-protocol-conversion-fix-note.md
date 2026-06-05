---
doc_type: issue-fix
issue: 2026-06-06-grok-protocol-conversion
status: implemented
severity: P1
summary: 修复 Grok Responses 请求未转换即转发到 /v1/chat/completions 的协议不匹配问题
tags:
  - grok
  - responses
  - chat-completions
  - protocol-conversion
---

# Grok protocol conversion Fix Note

## 修复结果

- Grok 的 `/v1/responses` 分支现在会先把 Responses 请求转换成 Chat Completions 请求，再发往 grok2api `/v1/chat/completions`。
- Grok 的 Anthropic Messages 入口现在复用已有协议转换链路：Anthropic → Responses → Chat Completions，返回再转回 Anthropic。
- Grok 相关单测已同步到新的 `/v1/chat/completions` 路径。

## 根因

`backend/internal/service/grok_gateway_service.go` 的 `ForwardAsResponses` 目标地址切到了 `/v1/chat/completions`，但请求体仍是原始 Responses body，导致上游 Chat Completions 接口收到不匹配协议。

## 修改内容

- `backend/internal/service/grok_gateway_service.go`
  - `ForwardAsResponses` 使用 `apicompat.ResponsesToChatCompletionsRequest` 生成上游请求体。
  - 修正模型映射、stream usage、响应转换和 usage/reasoning 记录。
  - `ForwardAsCC` 改为复用现有 apicompat 转换链路，减少手写工具调用/SSE 转换风险。

- `backend/internal/service/grok_gateway_service_test.go`
  - 更新 Grok 默认上游响应为 Chat Completions 格式。
  - 更新 Anthropic 入口测试预期为 `/v1/chat/completions`。
  - 新增 Responses → Chat Completions 转换回归测试。

## 验证

- `git diff --check -- backend/internal/handler/gateway_handler_responses.go backend/internal/service/grok_gateway_service.go backend/internal/service/grok_gateway_service_test.go`
- `cd backend; go test ./internal/pkg/apicompat -run "TestResponsesToChatCompletions|TestChatCompletionsToResponses|TestChatCompletionsStreamRoundTrip|TestAnthropicToResponses" -count=1`
- `cd backend; go test ./internal/service ./internal/handler -run TestNonExistent -count=1`
- `cd backend; go test -tags unit ./internal/service -run "TestGrokGatewayService" -count=1`

以上均通过。
