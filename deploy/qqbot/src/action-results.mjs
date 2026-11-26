import { publicErrors } from './chat-policy.mjs';
import { sensitiveText } from './memory.mjs';
import { voiceCatalog } from './voice.mjs';

const groupId = value => typeof value === 'string' && /^\d{1,24}$/u.test(value) ? value : '';

// 只保存动作元数据；待发正文、人设及模型原始参数不进入结果表。
export function actionDetails(name, args) {
  if (name === 'send_group_message') return { group_id: groupId(args?.group_id) };
  return { updates: Array.isArray(args?.updates) ? args.updates.map(update => ({
    scope: ['group_defaults', 'group', 'private'].includes(update?.scope) ? update.scope : '',
    group_ids: Array.isArray(update?.group_ids) ? update.group_ids.map(groupId).filter(Boolean) : [],
    mode: ['text', 'voice'].includes(update?.mode) ? update.mode : '',
    voice: voiceCatalog.some(voice => voice.name === update?.voice) ? update.voice : '',
    prompt: typeof update?.prompt === 'string',
    reset: Array.isArray(update?.reset) ? update.reset.filter(field => ['mode', 'voice', 'prompt'].includes(field)) : []
  })) : [] };
}

export function actionFailure(error, secrets) {
  let reason = publicErrors[error.message] || String(error.message);
  for (const secret of secrets) if (secret) reason = reason.replaceAll(secret, '[redacted]');
  return sensitiveText(reason) ? '操作失败，错误信息含敏感内容，已隐藏。' : reason.slice(0, 300);
}

export function actionQueryDetails(name, result) {
  if (name === 'list_voices') return { voices: result.voices.map(voice => voice.name) };
  if (name === 'list_reply_groups') return { groups: result.groups.map(group => ({ group_id: group.group_id, label: sensitiveText(group.label || '') ? '[敏感备注已隐藏]' : group.label || '' })) };
  const rule = value => ({ mode: value.mode, voice: value.voice, custom_prompt: Object.hasOwn(value, 'prompt') });
  return { settings: {
    group_defaults: rule(result.settings.group_defaults),
    groups: Object.fromEntries(Object.entries(result.settings.groups).map(([id, value]) => [id, rule(value)])),
    private_users: Object.fromEntries(Object.entries(result.settings.private_users).map(([id, value]) => [id, rule(value)]))
  } };
}

export function actionReport(store, parent) {
  const dispatches = store.groupDispatches(parent);
  const actions = store.actions(parent);
  if (!dispatches.length && !actions.some(action => ['send_group_message', 'update_reply_settings'].includes(action.name))) return;
  const lines = dispatches.map(row => {
    const receipts = store.receipts(row.id);
    const mode = row.mode === 'voice' ? `语音（${row.voice}）` : '文字';
    const state = row.status === 'sent' && receipts.length ? `已全部发送${mode}`
      : row.status === 'partial' ? `${mode}仅部分内容取得回执，其余结果未确认；不要重复提交`
      : ['failed', 'interrupted'].includes(row.status) ? `${mode}未完成发送`
      : `${mode}发送结果未确认；请先核对目标群，不要重复提交`;
    const reason = actions.findLast(action => action.name === 'send_group_message' && action.details.group_id === row.scope.split(':')[2])?.reason;
    return `群 ${row.scope.split(':')[2]}：${state}${receipts.length ? '；QQ 消息 ID：' + receipts.join('、') : '；没有已确认回执'}${row.status === 'failed' && reason ? '；' + reason : ''}。`;
  });
  for (const action of actions) {
    if (!['send_group_message', 'update_reply_settings'].includes(action.name)) continue;
    if (action.name === 'send_group_message') {
      if (dispatches.some(row => row.scope.split(':')[2] === action.details.group_id)) continue;
      const target = action.details.group_id ? `群 ${action.details.group_id}` : '目标群';
      lines.push(`${target}：${action.status === 'failed' ? '未发送' : '发送结果未确认，请先核对，不要重复提交'}${action.reason ? '；' + action.reason : ''}。`);
      continue;
    }
    if (action.status === 'not_submitted') {
      lines.push(`后续设置请求未执行；${action.reason}。`);
      continue;
    }
    const updates = action.details.updates.map(update => {
      const target = update.scope === 'group_defaults' ? '群共享默认' : update.scope === 'private' ? '当前私聊' : update.group_ids.length ? '群 ' + update.group_ids.join('、') : '指定范围';
      const changes = [
        update.mode && `回复方式：${update.mode === 'voice' ? '语音' : '文字'}`,
        update.voice && `音色：${update.voice}`,
        update.prompt && '人设已更新',
        update.reset.length && `恢复继承：${update.reset.map(field => ({ mode: '回复方式', voice: '音色', prompt: '人设' })[field]).join('、')}`
      ].filter(Boolean);
      return target + (changes.length ? '（' + changes.join('；') + '）' : '');
    });
    if (['saved', 'saved_sync_pending'].includes(action.status)) {
      lines.push(`回复设置已保存：${updates.join('、')}。${action.status === 'saved' ? '后续回复按新设置执行。' : '运行时同步尚未确认，请查询当前设置，不要重复提交。'}`);
      continue;
    }
    lines.push(`回复设置${action.status === 'failed' ? '未保存' : '保存结果未确认，请查询当前设置，不要重复提交'}${action.reason ? '；' + action.reason : ''}。`);
  }
  const queries = actions.filter(action => !['send_group_message', 'update_reply_settings'].includes(action.name)).map(action => {
    const name = { list_voices: '音色目录', list_reply_groups: '群列表', get_reply_settings: '回复设置' }[action.name];
    if (action.status !== 'read') return `${name}查询${action.status === 'failed' ? '失败' : '尚未完成'}${action.reason ? '：' + action.reason : ''}。`;
    if (action.name === 'list_voices') return '本轮查询的可用音色：' + action.details.voices.join('、') + '。';
    if (action.name === 'list_reply_groups') return '本轮查询的已开放群：\n' + action.details.groups.map(group => `${group.group_id}${group.label ? '（' + group.label + '）' : ''}`).join('\n');
    const settings = action.details.settings;
    const format = (target, rule) => `${target}：${[
      rule.mode ? `回复方式：${rule.mode === 'voice' ? '语音' : '文字'}` : '回复方式：继承默认',
      rule.voice ? '音色：' + rule.voice : '音色：继承默认',
      rule.custom_prompt ? '已设置自定义人设' : '人设：继承默认'
    ].join('；')}`;
    return '本轮查询时的回复设置（以查询后保存结果为准）：\n' + [
      format('群共享默认', settings.group_defaults),
      ...Object.entries(settings.groups).map(([id, rule]) => format('群 ' + id, rule)),
      ...Object.values(settings.private_users).map(rule => format('当前私聊', rule))
    ].join('\n');
  });
  return [...new Set([...queries, ...lines])].join('\n');
}
