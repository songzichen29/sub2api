## 1. 后端配置结构扩展

- [ ] 1.1 在 `ContentModerationConfig` 结构体中添加 `Protocol` 字段（`string`，默认 `openai_moderation`）
- [ ] 1.2 在 `ContentModerationConfig` 结构体中添加 `SystemPrompt` 字段（`string`，用于 Anthropic 协议）
- [ ] 1.3 添加协议类型常量：`ContentModerationProtocolOpenAI = "openai_moderation"` 和 `ContentModerationProtocolAnthropic = "anthropic_messages"`
- [ ] 1.4 在 `ContentModerationConfigView` 中添加对应的展示字段
- [ ] 1.5 在 `UpdateContentModerationConfigInput` 中添加 `Protocol` 和 `SystemPrompt` 指针字段
- [ ] 1.6 修改 `defaultContentModerationConfig()` 设置默认协议为 `openai_moderation`
- [ ] 1.7 修改 `normalize()` 方法处理协议和提示词的默认值与校验

## 2. 后端 Anthropic 协议调用实现

- [ ] 2.1 创建 `callAnthropicModeration()` 函数，实现 Anthropic Messages API 调用逻辑
- [ ] 2.2 实现 Anthropic 认证头构建（`x-api-key` + `anthropic-version`）
- [ ] 2.3 实现 Anthropic 请求体构建（`model`、`messages`、`system` 字段）
- [ ] 2.4 实现 Anthropic 响应解析（从 `content[].text` 提取 JSON，解析 `flagged` 和 `category_scores`）
- [ ] 2.5 修改 `callModerationOnceWithInput()` 增加协议分发逻辑
- [ ] 2.6 实现默认系统提示词常量 `defaultAnthropicModerationPrompt`

## 3. 后端配置验证与更新

- [ ] 3.1 在 `UpdateConfig()` 中添加 `Protocol` 字段处理逻辑
- [ ] 3.2 在 `UpdateConfig()` 中添加 `SystemPrompt` 字段处理逻辑
- [ ] 3.3 在 `validateConfig()` 中添加协议类型验证
- [ ] 3.4 在 `validateConfig()` 中添加系统提示词长度验证（最大 4000 字符）
- [ ] 3.5 修改 `configView()` 传递新增字段到视图

## 3.5 输入摘要脱敏逻辑调整

- [ ] 3.5.1 修改 `buildLog()` 方法，当 `flagged=true` 时跳过 `redactContentModerationSecrets()` 调用
- [ ] 3.5.2 确保 `input_excerpt` 字段在被拦截时保留完整的原始用户输入
- [ ] 3.5.3 非拦截记录（flagged=false）仍保持原有脱敏逻辑
- [ ] 3.5.4 编写单元测试验证被拦截输入不脱敏、非拦截输入脱敏

## 4. 后端 Handler 层适配

- [ ] 4.1 在 `contentModerationConfigRequest` 中添加 `Protocol` 和 `SystemPrompt` 字段
- [ ] 4.2 在 `UpdateConfig` handler 中传递新字段到 service 层

## 5. 前端类型定义扩展

- [ ] 5.1 在 `riskControl.ts` 中添加 `ModerationProtocol` 类型（`'openai_moderation' | 'anthropic_messages'`）
- [ ] 5.2 在 `ContentModerationConfig` 接口中添加 `protocol` 和 `system_prompt` 字段
- [ ] 5.3 在 `UpdateContentModerationConfig` 接口中添加可选的 `protocol` 和 `system_prompt` 字段

## 6. 前端配置界面

- [ ] 6.1 在 `configForm` 响应式对象中添加 `protocol` 和 `system_prompt` 字段
- [ ] 6.2 在基本设置标签页添加协议类型下拉选择器（Select 组件）
- [ ] 6.3 当选择 `anthropic_messages` 协议时，显示系统提示词编辑器（textarea）
- [ ] 6.4 添加系统提示词字符计数器
- [ ] 6.5 修改 `applyConfig()` 函数处理新字段的赋值
- [ ] 6.6 修改 `saveConfig()` 函数在 payload 中包含新字段
- [ ] 6.7 添加协议类型的 i18n 翻译（中英文）

## 7. 前端 API Key 测试适配

- [ ] 7.1 确认现有 `testAPIKeys` 功能在 Anthropic 协议下正常工作
- [ ] 7.2 如需要，在测试请求中传递 `protocol` 和 `system_prompt` 参数

## 8. 测试验证

- [ ] 8.1 编写单元测试：验证 OpenAI Moderation 协议行为不变（回归测试）
- [ ] 8.2 编写单元测试：验证 Anthropic 协议请求格式正确
- [ ] 8.3 编写单元测试：验证 Anthropic 响应解析正确（包括 thinking block 处理）
- [ ] 8.4 编写单元测试：验证协议类型验证（无效协议被拒绝）
- [ ] 8.5 编写单元测试：验证系统提示词长度验证
- [ ] 8.6 进行手动集成测试：使用 DashScope 千问 API 进行实际审核测试

## 9. 文档与国际化

- [ ] 9.1 添加 `admin.riskControl.protocol` 相关 i18n 键（中/英文）
- [ ] 9.2 添加 `admin.riskControl.systemPrompt` 相关 i18n 键（中/英文）
- [ ] 9.3 添加协议选择的提示文本
