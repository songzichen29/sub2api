---
doc_type: issue-fix-note
issue: 2026-06-08-responses-input-file-content-parts
status: draft
root_cause_type: data-format
related: []
tags: [openai-responses, input-file, apicompat]
---

# Responses input_file content part 静默丢弃修复记录

## 1. 修复内容

本次修复针对 OpenAI Responses `content` 中的 `input_file` 文件型 part 在兼容转换链路中被静默忽略的问题。

改动范围：

- `backend/internal/pkg/apicompat/types.go`
  - `ResponsesContentPart` 增加 `file_id`、`filename`、`file_data`、`file_url`、`mime_type`、`detail` 字段。
  - `ChatContentPart` 增加 `file` 字段和 `ChatFile` 结构。
  - Anthropic source 结构补充 `file_id`、`url`，并允许 `media_type` / `data` 为空。
- `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
  - `input_file` 不再静默丢弃。
  - `file_data` 按 media type 转为 Anthropic `document` / `image`；文本类文件可解码后作为 text 块降级。
  - `file_id` 转为 Anthropic `document.source.type=file`。
  - `file_url` 转为 Anthropic `document.source.type=url`。
- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`
  - Responses → Chat fallback 保留 `input_file`，转成 Chat `type=file` content part。
  - 只有 URL 或 filename 且不能表示为 Chat file part 时，降级为文本说明，避免完全丢失。
- 测试：
  - `TestResponsesToAnthropicRequest_InputFileDataBecomesDocument`
  - `TestResponsesToAnthropicRequest_InputFileIDBecomesDocumentReference`
  - `TestResponsesToChatCompletionsRequest_InputFilePartPreserved`

## 2. 验证结果

已执行：

```bash
go test -count=1 ./internal/pkg/apicompat
```

结果：通过。

另外做了最小复现：`input_text + input_file + input_text` 转换到 Anthropic 后，中间文件块保留为：

```json
{
  "type": "document",
  "source": {
    "type": "base64",
    "media_type": "application/pdf",
    "data": "JVBERi0xLjQK"
  }
}
```

修复前该块会被静默丢弃。

## 3. 未覆盖风险

- 当前只运行了 `internal/pkg/apicompat` 包测试；仓库存在大量既有未合并/冲突状态，未执行全仓测试，避免把无关冲突混入本次验证。
- OpenAI `file_id` 和 Anthropic `file_id` 不是同一文件系统 ID；当请求经由 OpenAI API 原生 passthrough 时应保持原样，经由 Anthropic 转换链路时只有上游可识别对应 file id 才能真实消费。`file_data` 是最可靠的跨协议内容携带方式。
- 对未知二进制类型默认按 `application/pdf` document 处理，目的是避免静默丢弃；若上游拒绝某些格式，需要后续按上游能力做更细的显式错误或专门映射。

## 4. 结论

本次问题的核心行为已修复：Responses 文件型 content part 不会再在兼容转换阶段被静默忽略，已用单元测试和最小复现验证。
