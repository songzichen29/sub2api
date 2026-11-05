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

// Keep complete personal-query grammar aligned with natural_account.go.
// A keyword mention, quotation, third-party request or mutation stays in chat.
const personalQueryPrefix = '^(?:我(?:让|叫)你\\s*)?(?:(?:请问|请|麻烦|劳驾)(?:你)?\\s*)?(?:(?:帮|给|替)我\\s*)?(?:(?:查询|查看|查一查|查一下|查查|查下|查|看一看|看一下|看看|看下|看|告诉我)\\s*)?(?:(?:我的?|本人(?:的)?)\\s*)?';
const personalQuerySubjects = [
  '(?:(?:账户|账号|钱包)的?)?(?:余额|剩余额度)(?:(?:还有|还剩|剩下|剩|有|是)?多少(?:钱|美元)?|剩多少)?',
  '(?:(?:账户|账号|钱包)(?:里|中)?)?(?:还有|还剩|剩下|剩|有)多少(?:钱|美元)',
  '(?:今天|今日)(?:的)?(?:用量|使用量|使用情况|消耗|消费|花费)(?:(?:有|是|了)?多少)?',
  '(?:今天|今日)(?:一共|总共)?(?:用|使用|消耗|花费|花)了?多少(?:用量|费用|token|钱|美元)?',
  '(?:剩余|剩下|剩)?(?:的)?抽奖(?:次数|机会)(?:(?:还有|还剩|剩下|剩|有|是)?多少(?:次)?|剩多少(?:次)?)?',
  '(?:还有|还剩|剩下|剩|有)多少(?:次)?抽奖(?:次数|机会)?',
  '(?:累计)?(?:抽过|抽了|抽中了|中奖了|中奖)(?:多少|几)次(?:奖)?'
].map(subject => new RegExp(personalQueryPrefix + subject + '(?:呢|呀|啊|吧|吗|么)?$', 'iu'));

export function isPersonalQuery(event) {
  if (event.message.some(s => !['text', 'at'].includes(s.type))) return false;
  const text = messageText(event).replace(/[\s。.!！?？～~]+$/u, '');
  return personalQuerySubjects.some(pattern => pattern.test(text));
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
  // A request directed at another member must not query this sender's account.
  const mentions = event.message.filter(s => s.type === 'at').map(s => String(s.data?.qq));
  if (mentions.length && !mentions.includes(config.bot_id)) return 'drop';
  if (['清空', '清空对话', '聊天', '分身'].includes(command)) return 'chat';
  if (text.startsWith('/') || config.business_commands.includes(command)) return 'business';
  if (isPersonalQuery(event)) return 'business';
  return 'chat';
}

export function shouldObserveGroup(event, config) {
  return config.business_observation === true && event?.post_type === 'message' &&
    event.message_type === 'group' && String(event.self_id) === config.bot_id &&
    config.groups.includes(String(event.group_id)) && String(event.user_id) !== config.bot_id &&
    /^\d{1,24}$/u.test(String(event.user_id)) && /^-?\d{1,24}$/u.test(String(event.message_id)) &&
    Array.isArray(event.message) && event.message.every(s => s && typeof s === 'object' && typeof s.type === 'string');
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
