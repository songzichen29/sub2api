// 聊天历史不决定输出方式；每轮从已保存的规则计算有效设置。
export function effectiveReplySettings(config, input) {
  const saved = config.reply_settings;
  const rule = input.group
    ? { ...saved?.group_defaults, ...saved?.groups?.[input.chat] }
    : saved?.private_users?.[input.sender] || {};
  return {
    mode: rule.mode ?? 'text',
    voice: rule.voice ?? '默认',
    persona: Object.hasOwn(rule, 'prompt')
      ? rule.prompt + (config.knowledge_prompt || '')
      : config.persona || ''
  };
}
