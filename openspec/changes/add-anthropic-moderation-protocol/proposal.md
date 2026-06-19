## Why

风控中心当前硬编码使用 OpenAI Moderation API 协议（`/v1/moderations`），无法对接其他兼容 Anthropic Messages 协议的内容审核服务。用户希望使用阿里云 DashScope 的千问模型（通过 Anthropic 兼容接口）进行内容审核，以获得更好的中文内容理解能力和更灵活的审核策略定制。

## What Changes

- 在风控配置中新增**审核协议类型**选择（`openai_moderation` / `anthropic_messages`）
- 支持 Anthropic Messages 协议的 API 调用（端点 `/v1/messages`，认证方式 `x-api-key`）
- 支持配置**系统提示词**（system prompt），用于定义 LLM 的审核行为和输出格式
- 支持解析 Anthropic Messages API 响应中的 JSON 审核结果
- 前端配置界面增加协议类型选择、系统提示词编辑器、协议特有配置项
- 保持向后兼容：默认协议仍为 `openai_moderation`，现有配置无需修改

## Capabilities

### New Capabilities
- `anthropic-moderation-protocol`: Anthropic Messages 协议的内容审核支持，包括协议调用、提示词配置、响应解析

### Modified Capabilities
（无现有 spec 需要修改）

## Impact

**后端代码**
- `backend/internal/service/content_moderation.go`: 配置结构扩展、新增 Anthropic 协议调用函数
- `backend/internal/service/content_moderation_test.go`: 新增测试用例

**前端代码**
- `frontend/src/views/admin/RiskControlView.vue`: 配置对话框增加协议类型和提示词配置
- `frontend/src/api/admin/riskControl.ts`: 类型定义扩展

**API 兼容性**
- 配置 API 新增字段，旧字段保持兼容
- 现有用户配置不受影响（默认回退到 OpenAI Moderation 协议）

**依赖**
- 无新增外部依赖
- Anthropic 协议调用复用现有 `net/http` 客户端
