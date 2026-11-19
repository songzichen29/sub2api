export function chatAllowed(c, input) {
  return c.enabled !== false && !c.chat_disabled && input.bot === c.bot_id &&
    (input.group
      ? c.groups.includes(input.chat) && !c.group_policies?.some(p => p.group_id === input.chat && p.disable_chat)
      : input.chat === input.sender && c.private_users.includes(input.sender));
}

export function groupAdministrator(c, input) {
  return Boolean(input.group && (
    ['admin', 'owner'].includes(input.role) ||
    (c.owners || []).includes(input.sender) ||
    (c.group_admins || []).some(admin => admin.app_id === input.bot && admin.group_id === input.chat && admin.member_id === input.sender)
  ));
}

export const publicErrors = {
  moderation_blocked: '原消息已被防刷屏拦截，本次不生成或发送回复。',
  event_check_failed: '消息权限或防刷屏状态暂时无法确认，本次未执行，请稍后重试。',
  voice_explicitly_declined: '本条消息明确要求不用语音，请使用文字回复。',
  unknown_voice: '不支持该音色，请使用已配置的音色名称。',
  queue_full: '聊天队列已满，请稍后再发一次。',
  turn_expired: '这条消息等待或处理超时，请重新发送。',
  runtime_stopping: '聊天服务正在重启，请稍后重试。',
  rate_limited: '当前聊天次数或回复额度已用完，请稍后再试。',
  cooldown: '当前聊天冷却尚未结束，请稍后再试。',
  scope_denied: '当前群或私聊的聊天权限已关闭。',
  delivery_disabled: '当前会话或图片发送权限已关闭，本次回复已取消。',
  configuration_not_synchronized: '聊天配置暂未同步，请稍后重试。',
  previous_execution_unconfirmed: '这条消息已有执行记录，结果尚未确认，系统不会重复执行。',
  invalid_image: '图片格式、数量或大小不符合要求，请每次发送至多4张、单张不超过6MB、合计不超过12MB。',
  voice_text_missing: '请在语音合成命令后填写要朗读的文本，或引用一条文字消息后发送“语音合成”。',
  voice_quote_unavailable: '无法读取这条引用消息，请重新引用当前会话内的消息，或直接填写要朗读的文本。',
  voice_quote_text_missing: '引用消息中没有可朗读的文字，请引用文字消息，或在语音合成命令后填写文本。',
  voice_quote_too_long: '引用消息超过可读取的长度，本次未合成语音，请分段引用或直接填写要朗读的文本。',
  voice_request_failed: '语音合成接口调用失败，本次未发送语音，也未自动重试。',
  voice_sensitive_text: '模型回答包含疑似密钥或口令，本次未提交第三方语音合成服务。',
  voice_response_invalid: '语音合成接口没有返回可识别的 MP3/WAV 音频，本次未发送语音，请核对接口响应格式。',
  voice_response_too_large: '语音合成返回的单段音频超过6MB，本次未发送语音，请缩短文本。',
  message_exceeds_model_capacity: '消息超过当前模型容量，请缩短内容或分开发送。',
  model_unavailable: '本次模型调用失败，请稍后再试。',
  empty_model_response: '模型没有返回文字或图片，请稍后再试。',
  invalid_receipts: '图片或文字的发送回执无效，系统不会重复发送。',
  media_storage_full: '图片存储已达到容量上限，请管理员检查存储。'
};
