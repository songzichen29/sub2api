import { opaqueKey } from './policy.mjs';
import { chatAllowed } from './chat-policy.mjs';
import { sensitiveText } from './memory.mjs';
import { effectiveReplySettings } from './reply-settings.mjs';
import { replyVoice } from './voice.mjs';

export async function sendGroupMessage(args, context, { config, store, speech, sendPacket }) {
  const { input, signal } = context;
  if (input.group || input.proactive) throw new Error('跨群发送仅允许白名单用户在私聊中操作');
  if (typeof args.group_id !== 'string' || !/^\d{1,24}$/u.test(args.group_id)) throw new Error('请先查询群列表并指定准确群号');
  if (typeof args.text !== 'string' || !args.text.trim()) throw new Error('请提供要发送的完整文本');
  if (sensitiveText(args.text)) throw new Error('不能向群聊发送密钥或口令');
  if (args.mode !== undefined && !['text', 'voice'].includes(args.mode)) throw new Error('invalid_reply_mode');
  const target = { ...input, group: true, chat: args.group_id };
  const allowed = () => {
    signal?.throwIfAborted();
    const current = config.snapshot();
    if (!chatAllowed(current, input)) throw new Error('私聊发送权限已关闭');
    if (!chatAllowed(current, target)) throw new Error('目标群未开放或已关闭聊天');
    return current;
  };
  const c = allowed();
  // 同一真实私聊消息对同一群只有一个发送动作，换 call_id 不能绕过。
  const id = opaqueKey(context.id + ':send-group:' + args.group_id);
  const request = JSON.stringify([args.group_id, args.text, args.mode ?? null, args.voice ?? null]);
  const prior = store.groupDispatch(id);
  const result = (mode, voice, status) => ({
    group_id: args.group_id, mode, ...(mode === 'voice' ? { voice } : {}), status,
    message_ids: store.receipts(id),
    notice: status === 'sent' ? '全部内容已取得 QQ 消息回执。' : '发送未完整确认，不会自动重发；请先核对目标群，不能宣称全部发送成功。'
  });
  if (prior) {
    if (prior.request !== request) throw new Error('本条消息已向该群提交过其他内容，请核对结果；修改或继续发送请发新消息');
    return result(prior.mode, prior.voice, store.delivery(id)?.status || store.turn(id).status);
  }
  const settings = effectiveReplySettings(c, target);
  const mode = args.mode ?? settings.mode;
  const voice = replyVoice(args.voice ?? settings.voice);
  const scope = `${input.bot}:group:${args.group_id}`;
  store.beginGroupDispatch(id, scope, request, mode, voice.voice, context.id);
  let attempted = false;
  try {
    const audio = mode === 'voice' ? await speech({ ...voice, text: args.text }, { signal }) : [];
    for (const item of audio) store.addMedia(id, item);
    const characters = Array.from(args.text);
    const packets = mode === 'voice'
      ? audio.map(item => ({ type: 'record', data: { file: 'base64://' + item.data } }))
      : Array.from({ length: Math.ceil(characters.length / 1200) }, (_, index) => ({ type: 'text', data: { text: characters.slice(index * 1200, (index + 1) * 1200).join('') } }));
    if (!packets.length) throw new Error('语音合成没有返回音频');
    allowed();
    store.finish(id, 'completed', args.text);
    store.claimDelivery(id);
    for (const [index, packet] of packets.entries()) {
      allowed();
      attempted = true;
      const messageId = await sendPacket({ bot: input.bot, sender: input.sender, group_id: args.group_id, segment: packet }, { signal });
      store.recordReceipts(id, [messageId]);
      store.observe(scope, { event_id: `dispatch:${id}:${index}`, sender: input.bot, name: '机器人', role: 'assistant',
        text: packet.type === 'text' ? packet.data.text : `[已发送${voice.voice}语音]` });
    }
    if (mode === 'voice') store.observe(scope, { event_id: `dispatch:${id}:transcript`, sender: input.bot, name: '机器人', role: 'assistant', text: args.text });
    store.finishDelivery(id, 'sent');
    return result(mode, voice.voice, 'sent');
  } catch (error) {
    const status = store.receipts(id).length ? 'partial' : attempted ? 'unconfirmed' : 'failed';
    if (store.delivery(id)) store.finishDelivery(id, status === 'failed' ? 'unconfirmed' : status);
    if (!attempted) store.finish(id, 'failed');
    if (attempted) return { ...result(mode, voice.voice, status), error: String(error.message) };
    throw error;
  }
}

export function groupDispatchReport(store, parent) {
  const rows = store.groupDispatches(parent);
  if (!rows.length) return;
  return '本次私聊回复未完成，跨群操作的实际结果如下，请先核对目标群，不要重复提交：\n' + rows.map(row => {
    const receipts = store.receipts(row.id);
    const state = row.status === 'sent' ? '已全部发送' : row.status === 'partial' ? '仅部分内容取得回执，其余结果未确认' : row.status === 'failed' || row.status === 'interrupted' ? '未完成发送' : '发送结果未确认';
    return `群 ${row.scope.split(':')[2]}：${state}${receipts.length ? '；QQ 消息 ID：' + receipts.join('、') : '；没有已确认回执'}。`;
  }).join('\n');
}

export function groupPacketClient(token, base = 'http://router:6200') {
  return async (body, { signal } = {}) => {
    const response = await fetch(base + '/send-group', {
      method: 'POST', headers: { 'x-bridge-token': token, 'Content-Type': 'application/json' },
      body: JSON.stringify(body), signal: AbortSignal.any([AbortSignal.timeout(30000), ...(signal ? [signal] : [])]), redirect: 'error'
    });
    const value = await response.json();
    if (!response.ok) throw new Error(value.error || '跨群发送结果未确认');
    return value.message_id;
  };
}
