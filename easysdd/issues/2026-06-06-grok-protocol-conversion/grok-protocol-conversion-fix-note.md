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
- Grok 的 `/v1/responses` 流式输出现在按 Responses SSE 格式写出 `event: ...` + `data: ...`，Codex TUI 可识别增量文本事件。
- Grok Chat Completions → Responses 流式转换的 `response.output_item.added` 现在带合法 message `content`，Codex CLI 能先建立 active message，再展示后续 `response.output_text.delta`。
- Grok 上游 `base_url` 兼容 `https://host`、`https://host/v1`、`https://host/v1/chat/completions`，避免拼成 `/v1/v1/chat/completions`。
- Grok 相关单测已同步到新的 `/v1/chat/completions` 路径。
- 追加修复 Grok Responses 多轮对话：Chat Completions 上游的 `chatcmpl_*` 不再作为 Responses `id` 返回，统一对客户端暴露 `resp_*`；本地缓存 30 分钟续链上下文，下一轮带 `previous_response_id` 时会补齐历史 messages。
- 对 Grok Responses 上游未返回 usage 的 0 token 场景增加诊断日志，保留真实 usage，不伪造 token。
- 追加修复 Grok `/v1/chat/completions` 直通路径 usage 记录：流式请求会强制上游返回 usage；同步/流式都支持识别 `prompt_tokens/completion_tokens` 和 `input_tokens/output_tokens` 两类 usage 字段。

## 根因

`backend/internal/service/grok_gateway_service.go` 的 `ForwardAsResponses` 目标地址切到了 `/v1/chat/completions`，但请求体仍是原始 Responses body，导致上游 Chat Completions 接口收到不匹配协议。

后续排查发现同一路径还有两个流式展示相关问题：

- Grok Responses 流式转换后只写了 `data: {...}`，缺少 Responses SSE 的 `event: response.output_text.delta` 等事件名；Codex TUI 会收到 token/usage，但不识别文本事件。
- Codex CLI 0.137 会先把 `response.output_item.added.item` 解析成 `ResponseItem::Message` 作为 active item；旧转换里的 message item 省略了必填的 `content`，解析失败后后续 `response.output_text.delta` 没有 active item，因此界面仍不展示。
- `base_url` 若配置为带版本段的 `/v1`，旧逻辑会继续追加 `/v1/chat/completions`。

## 修改内容

- `backend/internal/service/grok_gateway_service.go`
  - `ForwardAsResponses` 使用 `apicompat.ResponsesToChatCompletionsRequest` 生成上游请求体。
  - 修正模型映射、stream usage、响应转换和 usage/reasoning 记录。
  - `ForwardAsCC` 改为复用现有 apicompat 转换链路，减少手写工具调用/SSE 转换风险。
  - `ForwardAsResponses` 流式事件统一用 `apicompat.ResponsesEventToSSE` 输出，保留最终 `data: [DONE]`。
  - `response.output_item.added` 的 message item 补齐 `content:[{"type":"output_text","text":""}]`，并确保 text 类 content part 即使为空也序列化 `text` 字段。
  - Grok 上游 URL 改用 OpenAI-compatible endpoint builder，正确处理带 `/v1` 的 base URL。
  - `ForwardAsResponses` 增加 `previous_response_id` 本地续链缓存：命中时把历史 messages 拼到当前请求前；找不到或 id 类型错误时返回明确 `400 invalid_request_error`，不再静默丢上下文。
  - Grok Responses usage 为 0 时记录 account/model/stream/previous_response_id/上游 usage 信号，方便区分上游未给 usage 和真实 0 token。
  - `ForwardAsChatCompletions` 流式直通时注入 `stream_options.include_usage=true`，并兼容 `data:` 无空格 SSE 行。
  - Grok Chat Completions usage 提取兼容 `usage.prompt_tokens/completion_tokens`、`usage.input_tokens/output_tokens`、`response.usage.*` 以及 cached token 明细；上游没给 usage 时只记诊断日志，不伪造 token。

- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`
  - Chat Completions → Responses 非流式响应统一输出 `resp_*`。
  - 流式转换保持同一个 `resp_*` response id，避免每个 chunk 生成不同 id，也避免暴露 `chatcmpl_*`。

- `backend/internal/service/grok_gateway_service_test.go`
  - 更新 Grok 默认上游响应为 Chat Completions 格式。
  - 更新 Anthropic 入口测试预期为 `/v1/chat/completions`。
  - 新增 Responses → Chat Completions 转换回归测试。
  - 新增 Responses 流式 SSE 事件名、active message item content 与 `/v1` base URL 回归测试。
  - 新增 Grok Responses 多轮 `previous_response_id` 续链回归测试。
  - 新增 Grok `/v1/chat/completions` 同步 Responses 风格 usage、流式 include_usage 注入和 usage 提取回归测试。

- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge_test.go`
  - 新增 `chatcmpl_*` 转 `resp_*` 的非流式/流式回归测试。

## 验证

- `git diff --check -- backend/internal/handler/gateway_handler_responses.go backend/internal/service/grok_gateway_service.go backend/internal/service/grok_gateway_service_test.go`
- `cd backend; go test ./internal/pkg/apicompat -run "TestResponsesToChatCompletions|TestChatCompletionsToResponses|TestChatCompletionsStreamRoundTrip|TestAnthropicToResponses" -count=1`
- `cd backend; go test ./internal/service ./internal/handler -run TestNonExistent -count=1`
- `cd backend; go test -tags unit ./internal/service -run "TestGrokGatewayService" -count=1`
- `cd backend; go test ./internal/pkg/apicompat -count=1`
- `cd backend; go test ./internal/handler -run TestNonExistent -count=1`
- `git diff --check`

以上均通过。

补充说明：`cd backend; go test -tags unit ./internal/handler -run TestNonExistent -count=1` 当前仍因既有 `payment_handler_resume_test.go` 调用 `service.NewPaymentService` 参数数量不匹配而构建失败，和本次 Grok 改动无关。
