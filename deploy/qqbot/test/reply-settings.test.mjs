import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { createBridge } from '../src/bridge.mjs';
import { executeCapability } from '../src/capabilities.mjs';
import { effectiveReplySettings } from '../src/reply-settings.mjs';
import { RuntimeConfig } from '../src/runtime-config.mjs';
import { replyVoice, voiceCatalog } from '../src/voice.mjs';

const privateInput = { bot: '123', group: false, chat: '789', sender: '789', message_id: '1', text: '以后用御姐语音' };
const groupInput = { ...privateInput, group: true, chat: '456', text: '你好' };
const audio = { mimeType: 'audio/mpeg', data: 'SUQzBAAAAAAAAA==' };

async function fixture(t, run = async () => '回答正文', dependencies = {}) {
  const root = mkdtempSync(join(tmpdir(), 'qq-reply-settings-'));
  const store = new Store(join(root, 'state.sqlite'), join(root, 'raw'));
  const value = { enabled: true, bot_id: '123', groups: ['456', '457'], private_users: ['789'],
    persona: '原人设\n公开知识', base_persona: '原人设', knowledge_prompt: '\n公开知识',
    model: 'test', model_api_key: 'key', context_window_tokens: 16384, max_output_tokens: 512,
    daily_chat_limit: 1000, total_daily_chat_limit: 1000, cooldown_seconds: 0,
    reply_settings: {}, reply_settings_version: 'v1' };
  const spoken = [], starts = [], calls = [];
  let sessionNumber = 0;
  const config = { snapshot: () => value, policy: () => value };
  const bridge = createBridge({ config, token: 'test', store, root,
    speech: async command => { spoken.push(command); return [audio]; },
    makeHarness: async options => {
      starts.push(options);
      const session = id => ({ id, run: async blocks => {
        const context = JSON.parse(blocks[0].text);
        calls.push(context);
        return { finalResponse: await run({ context, options, id }) };
      } });
      return { start: async () => {}, close: async () => {}, session: () => session('session-' + ++sessionNumber), resumeSession: async id => session(id) };
    }, ...dependencies
  });
  bridge.server.listen(0, '127.0.0.1');
  await once(bridge.server, 'listening');
  const post = async (path, body) => {
    const response = await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`, {
      method: 'POST', headers: { 'x-bridge-token': 'test', 'Content-Type': 'application/json' }, body: JSON.stringify(body)
    });
    return { status: response.status, ...await response.json() };
  };
  t.after(async () => { await bridge.close(); store.db.close(); rmSync(root, { recursive: true, force: true }); });
  return { value, config, store, spoken, starts, calls, post };
}

test('saved reply rules inherit per field, separate private chats and preserve public knowledge', () => {
  const config = { persona: '原人设\n知识', knowledge_prompt: '\n知识', reply_settings: {
    group_defaults: { mode: 'voice', voice: '御姐', prompt: '共同人设' },
    groups: { '456': { mode: 'text' }, '457': { prompt: '' } },
    private_users: { '789': { voice: '曼波', prompt: '私聊人设' } }
  } };
  assert.deepEqual(effectiveReplySettings(config, groupInput), { mode: 'text', voice: '御姐', persona: '共同人设\n知识' });
  assert.equal(effectiveReplySettings(config, { ...groupInput, chat: '457' }).persona, '\n知识');
  assert.deepEqual(effectiveReplySettings(config, privateInput), { mode: 'text', voice: '曼波', persona: '私聊人设\n知识' });
  assert.deepEqual(effectiveReplySettings(config, { ...privateInput, sender: '999' }), { mode: 'text', voice: '默认', persona: '原人设\n知识' });
});

test('ordinary replies use saved voice; group exceptions, temporary text and voice requests preserve defaults', async t => {
  const f = await fixture(t);
  f.value.reply_settings = { group_defaults: { mode: 'voice', voice: '御姐' }, groups: { '457': { mode: 'text' } } };
  const voice = await f.post('/turn', groupInput);
  assert.equal(voice.status, 200);
  assert.equal(voice.text, '');
  assert.equal(f.spoken[0].voice, '御姐');
  assert.equal(f.spoken[0].text, '回答正文');
  const text = await f.post('/turn', { ...groupInput, message_id: '2', text: '这次不用语音，文字回复' });
  assert.equal(text.text, '回答正文');
  assert.equal(f.spoken.length, 1);
  await f.post('/turn', { ...groupInput, message_id: '3', text: '用语音回答我' });
  assert.equal(f.spoken.at(-1).voice, '御姐');
  const exception = await f.post('/turn', { ...groupInput, chat: '457', message_id: '4' });
  assert.equal(exception.text, '回答正文');
  const privateReply = await f.post('/turn', { ...privateInput, text: '你好', message_id: '5' });
  assert.equal(privateReply.text, '回答正文');
  assert.equal(f.calls[0].can_manage_replies, false);
  assert.equal(f.calls.at(-1).can_manage_replies, true);
});

test('prompt changes replace only the affected runtime and keep delivered conversation memory', async t => {
  const f = await fixture(t);
  const original = await f.post('/turn', { ...groupInput, text: '我叫小明' });
  await f.post('/delivery/claim', { id: original.id });
  await f.post('/delivery/finish', { id: original.id, status: 'sent', message_ids: ['100'] });
  f.value.reply_settings = { groups: { '456': { prompt: '简短回答' } } };
  await f.post('/turn', { ...groupInput, message_id: '2', text: '还记得我吗' });
  assert.equal(f.starts.length, 2);
  assert.match(f.starts[1].env.DSH_SYSTEM_PROMPT, /^简短回答\n公开知识/u);
  assert.ok(f.calls[1].conversation_memory.messages.some(message => message.text === '我叫小明'));
  await f.post('/turn', { ...groupInput, chat: '457', message_id: '3' });
  assert.match(f.starts[2].env.DSH_SYSTEM_PROMPT, /^原人设\n公开知识/u);
});

test('voice catalog queries and temporary selection use the actual synthesis IDs', async () => {
  const context = { input: privateInput, replySettings: { voice: '御姐' } };
  const config = { snapshot: () => ({ enabled: true, bot_id: '123', groups: ['456'], private_users: ['789'] }) };
  const catalog = await executeCapability({ name: 'list_voices', args: {} }, context, { config });
  assert.deepEqual(catalog.voices, voiceCatalog);
  assert.ok(catalog.voices.length > 20);
  for (const voice of catalog.voices) assert.equal(replyVoice(voice.name).referenceId, voice.reference_id);
  await executeCapability({ name: 'set_reply_mode', args: { mode: 'voice' } }, context, { config });
  assert.equal(context.replyVoice.voice, '御姐');
  context.replyVoice = replyVoice('曼波');
  await executeCapability({ name: 'set_reply_mode', args: { mode: 'voice' } }, context, { config });
  assert.equal(context.replyVoice.voice, '曼波', '工具省略音色时保留本轮明确选择，而不是回到长期音色');
});

test('management writes bind the private sender, batch updates and refresh only after saving', async () => {
  const value = { enabled: true, bot_id: '123', groups: ['456'], private_users: ['789'] };
  const calls = [];
  const config = { snapshot: () => value, refresh: async () => { calls.push('refresh'); value.reply_settings = { group_defaults: { mode: 'voice', voice: '御姐' } }; } };
  const updates = [{ scope: 'group_defaults', mode: 'voice', voice: '御姐' }, { scope: 'group', group_ids: ['456'], mode: 'text' }];
  const siteTool = async body => { calls.push(body); return { status: 'saved', version: 'v2' }; };
  const args = { version: 'v1', updates, member_id: '999', group_id: '456', operation_id: 'fake' };
  const result = await executeCapability({ name: 'update_reply_settings', args, call_id: 'call-1' }, { input: privateInput }, { config, siteTool });
  assert.equal(result.status, 'saved');
  assert.deepEqual(calls, [{ tool: 'update_reply_settings', version: 'v1', updates, operation_id: '1:reply-settings', group_id: '', member_id: '789', message_id: '1' }, 'refresh']);
  await assert.rejects(executeCapability({ name: 'update_reply_settings', args, call_id: 'call-2' }, { input: groupInput }, { config, siteTool }), /私聊/u);
  await assert.rejects(executeCapability({ name: 'update_reply_settings', args: { ...args, updates: [{ scope: 'private', voice: '不存在的音色' }] }, call_id: 'call-3' }, { input: privateInput }, { config, siteTool }), /unknown_voice/u);
  value.private_users = [];
  await assert.rejects(executeCapability({ name: 'get_reply_settings', args: {} }, { input: privateInput }, { config, siteTool }), /停用/u);
  assert.equal(calls.length, 2);
});

test('save and refresh failures expose the actual state and never retry the mutation', async () => {
  const value = { enabled: true, bot_id: '123', groups: [], private_users: ['789'] };
  const config = { snapshot: () => value, refresh: async () => { throw new Error('network'); } };
  const request = { name: 'update_reply_settings', args: { version: 'v1', updates: [{ scope: 'private', mode: 'voice' }] }, call_id: 'call' };
  let calls = 0;
  await assert.rejects(executeCapability(request, { input: privateInput }, { config, siteTool: async () => { calls++; throw new Error('版本冲突'); } }), /版本冲突/u);
  assert.equal(calls, 1);
  await assert.rejects(executeCapability(request, { input: privateInput }, { config, siteTool: async () => { calls++; return { status: 'saved' }; } }), /已保存.*同步失败/u);
  assert.equal(calls, 2);
  const context = { input: privateInput };
  const siteTool = async body => { calls++; if (body.tool === 'get_reply_settings') return { version: 'v2' }; throw new Error('network'); };
  await assert.rejects(executeCapability(request, context, { config, siteTool }), /network/u);
  await assert.rejects(executeCapability({ ...request, call_id: 'retry' }, context, { config, siteTool }), /已提交过/u);
  assert.equal(calls, 3, '失败后更换 call_id 不会再次写入');
  assert.equal((await executeCapability({ name: 'get_reply_settings', args: {} }, context, { config, siteTool })).version, 'v2');
});

test('saved voice keeps the existing media-only reply path without synthesizing an empty answer', async t => {
  const f = await fixture(t, async ({ options, id }) => {
    const response = await fetch(options.env.QQBOT_TOOL_URL, {
      method: 'POST', headers: { 'x-tool-token': options.env.QQBOT_TOOL_TOKEN, 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'send_meme', args: { keyword: '开心' }, call_id: 'meme', session: id })
    });
    assert.equal(response.status, 200);
    return '';
  });
  f.value.memes_enabled = true;
  f.value.reply_settings = { group_defaults: { mode: 'voice', voice: '御姐' } };
  const png = { mimeType: 'image/png', data: 'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg==' };
  f.store.collectMeme('123:group:456', png, '开心');
  const result = await f.post('/turn', { ...groupInput, text: '发个开心表情' });
  assert.equal(result.status, 200);
  assert.equal(result.text, '');
  assert.equal(result.media[0].mimeType, 'image/png');
  assert.equal(f.spoken.length, 0);
});

test('a management tool round saves group rules without redirecting the private reply, then ordinary group messages use them', async t => {
  const siteCalls = [];
  const saved = { group_defaults: { mode: 'voice', voice: '御姐' }, groups: { '457': { mode: 'text' } } };
  const f = await fixture(t, async ({ context, options, id }) => {
    if (!context.can_manage_replies) return '群里继续聊天';
    const tool = async (name, args, call_id) => {
      const response = await fetch(options.env.QQBOT_TOOL_URL, {
        method: 'POST', headers: { 'x-tool-token': options.env.QQBOT_TOOL_TOKEN, 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, args, call_id, session: id })
      });
      assert.equal(response.status, 200);
      return response.json();
    };
    const current = await tool('get_reply_settings', {}, 'query');
    const result = await tool('update_reply_settings', { version: current.version,
      updates: [{ scope: 'group_defaults', mode: 'voice', voice: '御姐' }, { scope: 'group', group_ids: ['457'], mode: 'text' }] }, 'save');
    assert.equal(result.status, 'saved');
    return '群默认已改成御姐语音，工作群保留文字。';
  }, { siteTool: async body => {
    siteCalls.push(body);
    return body.tool === 'get_reply_settings' ? { version: 'v1', settings: {} } : { status: 'saved', version: 'v2', settings: saved };
  } });
  f.config.refresh = async () => { f.value.reply_settings = saved; f.value.reply_settings_version = 'v2'; };
  const result = await f.post('/turn', { ...privateInput, text: '所有群以后用御姐语音，工作群继续用文字' });
  assert.equal(result.status, 200);
  assert.match(result.text, /已改成御姐/u);
  assert.equal(f.store.turn(result.id).scope, '123:private:789');
  assert.ok(siteCalls.every(call => call.group_id === '' && call.member_id === '789'));
  const group = await f.post('/turn', { ...groupInput, message_id: '2' });
  assert.equal(group.text, '');
  assert.equal(f.spoken.at(-1).voice, '御姐');
  const exception = await f.post('/turn', { ...groupInput, chat: '457', message_id: '3' });
  assert.equal(exception.text, '群里继续聊天');
  await f.post('/turn', { ...privateInput, text: '所有群以后用御姐语音，工作群继续用文字' });
  assert.equal(siteCalls.length, 2, '重复 QQ 消息不重新执行管理工具');
});

test('configuration refreshes serialize so a pending old poll cannot replace a saved update', async () => {
  let release;
  const blocked = new Promise(resolve => { release = resolve; });
  const requests = [];
  const runtime = new RuntimeConfig({ bot_id: '123', site_config_url: 'https://example.invalid/runtime/config' }, 'token', {
    fetcher: async (url, options) => {
      if (options.method === 'POST') return { ok: true };
      requests.push('config');
      const number = requests.length;
      if (number === 1) await blocked;
      return { ok: true, json: async () => ({ bot_id: '123', groups: [], private_users: ['789'], revision: String(number).repeat(64),
        persona: 'secret prompt', base_persona: 'secret prompt', knowledge_prompt: 'knowledge', reply_settings: { private_users: { '789': { prompt: 'private prompt' } } } }) };
    }
  });
  const oldPoll = runtime.refresh();
  const afterSave = runtime.refresh();
  assert.equal(requests.length, 1);
  release();
  await Promise.all([oldPoll, afterSave]);
  assert.equal(runtime.snapshot().revision, '2'.repeat(64));
  assert.equal(runtime.policy().reply_settings, undefined);
  assert.equal(runtime.policy().base_persona, undefined);
  assert.equal(runtime.policy().knowledge_prompt, undefined);
});
