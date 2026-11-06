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
  message_exceeds_model_capacity: '消息超过当前模型容量，请缩短内容或分开发送。',
  model_unavailable: '本次模型调用失败，请稍后再试。',
  empty_model_response: '模型没有返回文字或图片，请稍后再试。',
  invalid_receipts: '图片或文字的发送回执无效，系统不会重复发送。',
  media_storage_full: '图片存储已达到容量上限，请管理员检查存储。'
};
