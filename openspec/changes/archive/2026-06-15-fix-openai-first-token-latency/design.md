## Context

当前仓库有两套相关证据：

- fork 源 `upstream/main`：
  - HTTP `/v1/responses` streaming 在 `handleStreamingResponse` / `handleStreamingResponsePassthrough` 中用 `openAIStreamDataStartsClientOutput(data,eventType)` 判断首字。该函数只排除空 data、`response.failed` 和 preamble 事件 `response.created` / `response.in_progress`，因此 `response.output_item.added`、`response.output_item.done`、非空普通输出事件都会触发 `firstTokenMs`。
  - 普通 HTTP streaming 中 `firstTokenMs` 在下游写入/flush 之后记录，容易受客户端、代理缓冲、flush backpressure 影响。
  - WebSocket v1 的 `isOpenAIWSTokenEvent` 排除 preamble 和 `response.output_item.added/done`，但把 `.delta`、`response.output_text*`、`response.output*` 视为 token。
  - WebSocket v2 relay 的 `isTokenEvent` 更宽，除上述外还把 `response.completed` / `response.done` 当 token，存在“无真实 delta 时把终止时间当首 token”的误报风险。
- 当前分支：
  - 新增 `upstreamFirstEventMs`、debug timing comment、preamble 定时释放和慢首字诊断，能够区分“上游首包”和“真实首 token”。
  - HTTP streaming 的 `firstTokenMs` 已移动到下游写入/flush 前记录，避免被下游阻塞放大。
  - `openAIStreamDataStartsFirstToken` 和 WS v1/v2 的 token 判定收窄为“事件类型包含 `.delta` 且 data.delta 非空”。这修复了 fork 源把 preamble、output item lifecycle 或终止事件误报为首 token 的问题，但也会漏掉或延后一些 Responses API 的有效输出进度，例如非文本输出项、工具调用阶段、图片输出完成、或未来新增的非 `.delta` 有效输出事件。

当前用户反馈“当前分支的 token 耗时特别久”，更可能不是上游真的首包慢，而是指标语义从“首个客户端可见非 preamble 输出”变成“首个非空 `.delta` token”。如果请求先产生大量 reasoning / tool / item lifecycle，或者模型长期只发送非 token 事件，则 `first_token_ms` 会显著变大，而 fork 源会更早记录但语义偏宽。

## Goals / Non-Goals

**Goals:**

- 给 `first_token_ms` 建立一致、可测试、跨 HTTP streaming / OAuth passthrough / WS v1 / WS v2 relay 的定义。
- 保留当前分支已修复的准确性：不得把 `response.created`、`response.in_progress`、`response.output_item.added/done`、`response.completed/done/failed/...`、`[DONE]`、空 delta、debug timing comment 记为首 token。
- 避免当前分支过窄：首 token 事件分类必须覆盖所有可证明包含非空输出增量或用户可感知输出进度的 Responses 事件，而不是机械只看 `.delta` 字符串。
- 保留 `upstream_first_event_ms` 作为独立诊断维度，用它解释“上游已首包但首 token 慢”的问题。
- 避免下游写入/flush/backpressure 影响 `first_token_ms`，首 token 应在识别到上游有效 token 事件时立即记录。

**Non-Goals:**

- 不改变计费 token 的计算，不把 `first_token_ms` 与 usage token 数量混用。
- 不移除 pre-output failover、preamble 缓冲、preamble 定时释放或 debug timing comment。
- 不引入新的数据库破坏性迁移；已有 `first_token_ms` / `upstream_first_event_ms` 字段继续复用。
- 不把 `upstream_first_event_ms` 伪装成 `first_token_ms`，避免重新引入 fork 源“看起来快但语义不准”的误报。

## Decisions

### 1. 将首 token 判定抽成共享语义函数

建立一个 Responses 事件判定函数，例如 `openAIResponsesEventStartsFirstToken(data,eventType)`，供 HTTP streaming、passthrough、WS v1、WS v2 relay 复用或保持等价测试。它应：

- 先标准化事件类型：优先 JSON `type`，为空时使用 SSE `event:` 行携带的类型。
- 排除 preamble：`response.created`、`response.in_progress`。
- 排除终止/状态事件：`response.completed`、`response.done`、`response.failed`、`response.incomplete`、`response.cancelled`、`response.canceled`、`[DONE]`。
- 排除生命周期事件：`response.output_item.added`、`response.output_item.done`，除非后续确认 payload 中直接携带了用户可感知的最终输出且无更细粒度事件可用。
- 接受非空增量事件：至少包括 `response.output_text.delta`、`response.function_call_arguments.delta`、`response.reasoning_summary_text.delta`、其它包含非空 `delta` 的 `*.delta` 事件。
- 对没有 `delta` 字段但确实表示输出进度的事件，必须显式列入白名单并检查非空内容字段，避免 fork 源式“非 preamble 都算首 token”的过宽规则。

备选方案是直接恢复 fork 源 `openAIStreamDataStartsClientOutput`。不采用，因为它会把 `output_item.added/done` 和部分状态事件过早计入 TTFT，并且普通 HTTP 旧实现还可能受 flush 影响。

### 2. 保持 `upstream_first_event_ms` 与 `first_token_ms` 双指标

`upstream_first_event_ms` 记录首个上游 SSE/WS 事件到达，`first_token_ms` 只记录首个有效输出增量。慢首字排查以两者差值为主：

- `upstream_first_event_ms` 很小、`first_token_ms` 很大：上游已经响应，但模型在首个有效输出前长时间 reasoning、工具准备或发送非 token 事件。
- 两者都大：上游连接、排队、模型首包整体慢。
- `upstream_first_event_ms` 有值、`first_token_ms` 为空：上游没有产生可计首 token 的有效输出事件，不能用总耗时或终止事件兜底。

### 3. 首 token 记录点必须早于下游写入/flush

当前分支在 HTTP streaming 中已将 `firstTokenMs` 记录移到下游写入前，这是正确方向。实现时应保留这一点：识别到有效首 token 后立即 `time.Since(startTime)`，然后再执行 model 替换、tool 修正、buffered writer 写入、flush 或 client disconnect 处理。

### 4. Preamble 释放只改善客户端“有无响应”，不改变首 token

`openAIResponsesPreambleMaxPendingAge` 释放 `response.created` 等 preamble 能让客户端知道请求未卡死，但不得把释放时刻写入 `firstTokenMs`。如果客户端需要诊断上游首包，应使用 opt-in debug timing comment 或 `upstream_first_event_ms`，而不是复用 `first_token_ms`。

### 5. 测试以事件序列而不是具体网络时间为核心

测试应构造可控 SSE/WS 事件序列，验证：

- fork 源误报场景：只有 `response.created` / `response.output_item.added` / `response.completed` 时，`firstTokenMs` 仍为 nil。
- 当前分支长首字场景：非空有效 delta 到达时立即记录，且记录早于慢下游 writer 造成的延迟。
- 事件类型来自 SSE `event:` 行、JSON `type` 为空时仍能识别。
- 空 delta 不记录；随后非空 delta 记录。
- WS v1 和 WS v2 relay 不能把 terminal event 当 token。

## Risks / Trade-offs

- [Risk] 首 token 的“有效输出进度”边界与 OpenAI Responses API 后续新增事件类型有关。→ Mitigation：用显式白名单 + 非空 payload 检查，并在测试中加入 unknown `.delta` 可接受、unknown 非 delta 不接受的规则。
- [Risk] 对 `function_call_arguments.delta` 计入首 token 会让工具调用请求 TTFT 变短，但这不是用户可见文本。→ Mitigation：把 spec 定义为“首个有效输出增量/进度”而不是“首个可见文本字符”；如后续需要可再新增 `first_text_token_ms`。
- [Risk] 修正后历史 `first_token_ms` 统计口径与新数据不同。→ Mitigation：不回填历史数据，在设计和注释中明确 `upstream_first_event_ms` 与 `first_token_ms` 的含义，统计层按时间范围自然过渡。
- [Risk] 多路径实现分叉导致未来再次不一致。→ Mitigation：共享辅助函数或用相同表驱动测试覆盖 HTTP、passthrough、WS v1、WS v2 relay。
