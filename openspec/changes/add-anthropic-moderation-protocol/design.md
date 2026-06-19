## Context

风控中心（Content Moderation Service）当前硬编码使用 OpenAI Moderation API 协议：
- 端点：`/v1/moderations`
- 认证：`Authorization: Bearer {api_key}`
- 请求格式：`{"model": "...", "input": "..."}`
- 响应格式：`{"results": [{"flagged": bool, "category_scores": {...}}]}`

用户需要支持阿里云 DashScope 的千问模型（通过 Anthropic 兼容接口）进行内容审核，以获得：
- 更好的中文内容理解能力
- 可定制的审核策略（通过 system prompt）
- 更灵活的模型选择

经测试验证，DashScope Anthropic 兼容接口可以正确返回结构化审核结果。

## Goals / Non-Goals

**Goals:**
- 支持 `anthropic_messages` 协议类型，调用 Anthropic Messages API 进行内容审核
- 支持配置 system prompt 定义 LLM 审核行为
- 前端配置界面支持协议选择和提示词编辑
- 保持与现有 OpenAI Moderation 协议的完全向后兼容
- 复用现有的 API Key 轮询、健康检测、冻结机制

**Non-Goals:**
- 不支持除 OpenAI Moderation 和 Anthropic Messages 之外的其他协议
- 不改变现有的阈值判定逻辑（仍然使用 `category_scores` 与阈值对比）
- 不支持图片审核（Anthropic 协议的图片支持留作后续扩展）
- 不实现 OpenAI 与 Anthropic 协议的自动切换/降级

## Decisions

### Decision 1: 协议类型作为配置字段

**选择**: 在 `ContentModerationConfig` 中新增 `protocol` 字段（`string`），值为 `openai_moderation` 或 `anthropic_messages`。

**理由**: 
- 简单直接，易于扩展
- 默认值为 `openai_moderation`，保证向后兼容
- 前端可以用下拉框切换

**替代方案**: 使用接口类型抽象（`ModerationClient` interface），但会增加代码复杂度，且当前只需要支持两种协议。

### Decision 2: System Prompt 存储与默认值

**选择**: 
- 在配置中新增 `system_prompt` 字段（`string`）
- 提供内置默认提示词，当用户未配置时使用
- 前端提供提示词编辑器，限制 4000 字符

**理由**:
- 默认提示词确保开箱即用
- 允许高级用户自定义审核标准
- 字符限制防止配置过大

### Decision 3: 响应解析策略

**选择**: 
- 在 `callModerationOnceWithInput` 中根据协议类型分发到不同的调用函数
- Anthropic 协议使用 `callAnthropicModeration` 函数
- 解析 `content` 数组中的 `text` 类型块，提取 JSON

**理由**:
- 最小化代码改动，不影响现有逻辑
- 协议差异封装在各自的调用函数中
- 统一的 `moderationAPIResult` 返回类型便于后续处理

### Decision 4: 认证方式适配

**选择**:
- OpenAI: `Authorization: Bearer {key}`
- Anthropic: `x-api-key: {key}` + `anthropic-version: 2023-06-01`

**理由**:
- 遵循各协议的官方认证规范
- DashScope 兼容接口要求此格式

### Decision 5: 错误处理与降级

**选择**:
- Anthropic 响应解析失败时，记录错误日志并放行请求（与现有 OpenAI 错误处理一致）
- 不支持协议级别的自动降级（如 Anthropic 失败后尝试 OpenAI）

**理由**:
- 保持现有行为一致性
- 避免复杂的降级逻辑
- 用户可以配置多个 API Key 在协议内轮询

### Decision 6: 被拦截输入不进行脱敏

**选择**:
- 当内容被标记为违规（flagged=true）时，`input_excerpt` 字段保留完整的原始用户输入
- 不对 URL、Token、API Key 等敏感模式进行脱敏
- 仅在非违规记录（flagged=false）中应用脱敏处理

**理由**:
- 管理员需要看到完整的违规输入以进行审计和判定
- 脱敏会破坏审核上下文，影响管理员判断
- 风控日志仅对管理员可见，不属于外部泄露风险
- 与协议类型无关，OpenAI 和 Anthropic 协议统一行为

**替代方案**: 仅在 `anthropic_messages` 协议下不脱敏 → 拒绝，因为行为不一致会增加理解成本

## Risks / Trade-offs

**[Risk] LLM 响应格式不稳定**
→ 缓解：使用明确的 system prompt 约束输出格式；解析失败时放行并记录日志；前端提供测试功能让用户验证

**[Risk] Anthropic 协议响应延迟较高**
→ 缓解：这是 LLM 审核的固有限制，与专用分类模型相比延迟更高；在 `observe` 模式下影响较小；`pre_block` 模式需要用户接受此权衡

**[Risk] System Prompt 注入风险**
→ 缓解：System Prompt 由管理员配置，不受用户输入影响；用户输入在 `messages` 数组中作为 `user` 角色传入

**[Trade-off] 配置复杂度增加**
→ 接受：新增协议类型和提示词配置会增加用户理解成本；通过合理的默认值和 UI 提示缓解

**[Trade-off] 代码分支增加**
→ 接受：协议分发会增加代码复杂度；通过清晰的函数命名和注释缓解

## Migration Plan

1. **后端变更**
   - 扩展 `ContentModerationConfig` 结构体
   - 新增 `callAnthropicModeration` 函数
   - 修改 `callModerationOnceWithInput` 增加协议分发
   - 新增配置验证逻辑

2. **前端变更**
   - 扩展类型定义
   - 增加协议选择器 UI
   - 增加 system prompt 编辑器
   - 条件渲染协议特有配置项

3. **部署**
   - 无数据库迁移（配置存储在 settings 表 JSON 中）
   - 旧配置自动兼容（`protocol` 字段缺失时默认 `openai_moderation`）

4. **回滚**
   - 回滚代码即可，无需数据恢复
   - 用户配置的 `protocol` 和 `system_prompt` 字段在旧版本中会被忽略

## Open Questions

（无待解决问题，设计方案已明确）
