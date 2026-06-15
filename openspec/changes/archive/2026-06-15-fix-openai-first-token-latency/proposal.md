## Why

当前分支在 OpenAI `/v1/responses` 流式链路中把“真实首 token”从 fork 源的“首个非 preamble 客户端输出事件”改成了“非空 `.delta` 事件”，同时新增了 preamble 缓冲/定时释放与慢首字诊断。这样避免了 `response.created`、`response.output_item.added/done`、终止事件等被误记为首 token，但也会让没有早期文本 delta、仅产生工具调用/图片/结构化输出/长时间 reasoning 的请求表现为首字 token 特别久，甚至 `first_token_ms` 为空或接近后段事件时间。

需要把首字 token 语义、各流式传输路径的事件判定、诊断指标与调度统计统一起来，既保留修正后的准确性，又避免当前分支把“首个有效输出/可见进度”延迟误判为模型首字耗时。

## What Changes

- 明确定义并区分两个指标：`upstream_first_event_ms` 表示上游首个 SSE/WS 事件到达；`first_token_ms` 表示首个真实、非空、可计为输出进度的 token/增量事件。
- 对比 fork 源逻辑后修正当前分支的首 token 事件分类：保留排除 `response.created`、`response.in_progress`、终止事件和空 delta 的规则，同时覆盖 Responses API 中有效的文本、reasoning、function call arguments、tool call、图片等非空输出增量。
- 避免把 preamble 定时释放、debug timing comment、下游 flush/backpressure 或客户端断连计入 `first_token_ms`。
- 补齐普通 HTTP streaming、OAuth passthrough、Responses WebSocket v1/v2 relay 的一致性测试，覆盖 fork 源误报场景和当前分支长首字场景。
- 保持 OpenSpec/运行时变更最小化，不改外部 API 契约；新增或复用诊断字段只用于更准确的 usage、ops、调度与问题定位。

## Capabilities

### New Capabilities
- `openai-stream-first-token-metrics`: 定义 OpenAI 流式转发中首个上游事件、首个有效输出增量、首 token 记录、慢首字诊断与调度统计之间的一致行为。

### Modified Capabilities

## Impact

- 主要影响后端 OpenAI 流式转发与指标记录：`backend/internal/service/openai_gateway_service.go`、`backend/internal/service/openai_ws_forwarder.go`、`backend/internal/service/openai_ws_v2/passthrough_relay.go`、`backend/internal/service/usage_log.go`。
- 影响 handler 中写入 ops latency 和调度结果的链路：`backend/internal/handler/openai_gateway_handler.go`、`backend/internal/handler/openai_chat_completions.go`、`backend/internal/handler/openai_images.go`。
- 影响 usage/ops 查询中 `first_token_ms` 与 `upstream_first_event_ms` 的解释，不引入数据库破坏性变更。
- 需要新增/调整相关 Go 单元测试，重点验证首 token 不被 preamble/终止事件误报，也不因只认 `.delta` 而漏掉实际有效输出进度。
