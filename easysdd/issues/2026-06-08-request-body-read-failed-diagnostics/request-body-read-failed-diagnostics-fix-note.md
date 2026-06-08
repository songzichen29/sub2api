---
doc_type: issue-fix
status: completed
issue: request-body-read-failed-diagnostics
created_at: 2026-06-08
updated_at: 2026-06-08
---

# Failed to read request body 诊断修复记录

## 问题

OpenAI / Responses 请求偶发返回：

```json
{"error":{"message":"Failed to read request body","type":"invalid_request_error"}}
```

该错误发生在 handler 调用 `ReadRequestBodyWithPrealloc(c.Request)` 阶段，还未进入 JSON 解析、模型转发或 `input_file` 兼容转换。

## 根因判断

服务端无法恢复客户端或代理未完整发送的 HTTP body，但原实现存在两个排障问题：

1. 所有非超限读取错误都统一返回 `Failed to read request body`，无法区分坏压缩、unsupported `Content-Encoding`、普通读流失败。
2. 服务端没有记录安全诊断字段，下一次只能靠猜测判断是客户端断流、chunked 未完成、Content-Length 不匹配，还是压缩头错误。

## 修复内容

- `backend/internal/pkg/httputil/body.go`
  - 新增 `RequestBodyReadError`。
  - 分类：
    - `read`
    - `decode_content_encoding`
    - `unsupported_content_encoding`
  - 保持 `errors.As` 能继续识别 `*http.MaxBytesError`。

- `backend/internal/handler/request_body_read_error.go`
  - 新增统一 handler helper。
  - 非超限错误会记录：
    - `request_id`
    - `client_request_id`
    - `client_ip`
    - `method`
    - `path`
    - `content_length`
    - `content_type`
    - `content_encoding`
    - `transfer_encoding`
    - `body_read_error_kind`
  - 不记录请求体内容，避免泄漏文件内容或 prompt。

- 接入主要 OpenAI / Responses / Chat Completions 入口：
  - `backend/internal/handler/openai_gateway_handler.go`
  - `backend/internal/handler/openai_chat_completions.go`
  - `backend/internal/handler/openai_embeddings.go`
  - `backend/internal/handler/openai_images.go`
  - `backend/internal/handler/gateway_handler.go`
  - `backend/internal/handler/gateway_handler_responses.go`
  - `backend/internal/handler/gateway_handler_chat_completions.go`

## 行为变化

- 请求体超限：仍返回 `Request body too large...`。
- 不支持的 `Content-Encoding`：返回 `Unsupported Content-Encoding: ...`。
- 压缩声明与实际内容不匹配：返回 `Failed to decode request body with Content-Encoding: ...`。
- 客户端/代理断流等普通读失败：仍返回 `Failed to read request body`，但服务端日志会带底层错误和请求头诊断字段。

## 验证

```powershell
cd backend
go test -count=1 ./internal/pkg/httputil ./internal/handler -run 'TestReadRequestBodyWithPrealloc|TestRequestBodyReadError'
go test -count=1 ./internal/pkg/apicompat -run 'TestResponsesToChatCompletionsRequest_InputFilePartPreserved|TestResponsesToAnthropicRequest_InputFile(DataBecomesDocument|IDBecomesDocumentReference)'
```

结果均通过。

## 剩余边界

如果客户端或中间代理确实提前断开连接，服务端无法补齐没收到的 body；本次修复使该类问题可诊断、可关联、可定位，而不是静默合并成同一个泛化错误。
