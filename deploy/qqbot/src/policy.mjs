import { createHash, timingSafeEqual } from 'node:crypto';

export function authorized(actual, expected) {
  if (typeof actual !== 'string' || !expected) return false;
  const a = Buffer.from(actual), b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}

export function messageText(event) {
  return Array.isArray(event.message)
    ? event.message.filter(s => s.type === 'text').map(s => s.data?.text ?? '').join('').trim()
    : '';
}

export function routeEvent(event, config) {
  if (!event || typeof event !== 'object' || Array.isArray(event)) return 'drop';
  if (String(event.self_id) !== config.bot_id) return 'drop';
  if (event.post_type !== 'message') return 'business';
  if (!Array.isArray(event.message) || event.message.some(s => !s || typeof s !== 'object' || typeof s.type !== 'string')) return 'drop';
  if (String(event.user_id) === config.bot_id) return 'drop';
  const text = messageText(event);
  const command = text.replace(/^\//u, '').split(/\s/u)[0];
  if (event.message_type === 'private') {
    // Binding codes and email addresses never enter the conversational store.
    if (!config.private_users.includes(String(event.user_id)) ||
        /^(?:\/?(?:绑定|确认|解绑)|[a-z\d]{6,16}$|[a-z\d_-]{24}$|[^\s@]+@[^\s@]+\.[^\s@]+$)/iu.test(text)) return 'business';
    return 'chat';
  }
  if (event.message_type !== 'group' || !config.groups.includes(String(event.group_id))) return 'drop';
  if (['清空', '清空对话', '聊天', '分身'].includes(command)) return 'chat';
  if (text.startsWith('/') || config.business_commands.includes(command)) return 'business';
  if (/(?:查|看|多少|剩|我的).{0,8}(?:余额|用量|抽奖次数)|(?:余额|用量|渠道状态|渠道价格|签到规则|转账规则)/u.test(text)) return 'business';
  // An explicit mention of someone else does not request this bot's attention.
  const mentions = event.message.filter(s => s.type === 'at').map(s => String(s.data?.qq));
  if (mentions.length && !mentions.includes(config.bot_id)) return 'drop';
  return 'chat';
}

export function scopeKey(input) {
  for (const field of ['bot', 'sender', 'message_id']) {
    if (typeof input[field] !== 'string' || !/^[a-zA-Z0-9_.:-]{1,128}$/u.test(input[field])) throw new Error('invalid_identity');
  }
  if (typeof input.group !== 'boolean' || typeof input.chat !== 'string' || !/^\d{1,24}$/u.test(input.chat)) throw new Error('invalid_chat');
  // Group conversations isolate each member; autonomous context can be supplied separately.
  return [input.bot, input.group ? 'group' : 'private', input.chat, input.sender].join(':');
}

export function opaqueKey(value) { return createHash('sha256').update(value).digest('hex'); }

export function boundedText(text, max = 4000) {
  if (typeof text !== 'string' || Array.from(text).length > max) throw new Error('invalid_text');
  return text.trim();
}
