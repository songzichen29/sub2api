import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { WebSocket, WebSocketServer } from 'ws';
import { Store } from '../src/store.mjs';
import { createRouter } from '../src/router.mjs';
import { createBridge } from '../src/bridge.mjs';
import { sendGroupMessage, groupPacketClient } from '../src/group-message.mjs';
import { executeCapability } from '../src/capabilities.mjs';
import { opaqueKey } from '../src/policy.mjs';
import { replyVoice } from '../src/voice.mjs';

const input = { bot: '123', group: false, chat: '789', sender: '789', message_id: '1', text: '把通知发到 456 群' };
const audio = { mimeType: 'audio/mpeg', data: 'SUQzBAAAAAAAAA==' };

async function fixture(t, acknowledge, { policy } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'qq-group-message-'));
  const state = { store: new Store(join(root, 'state.sqlite'), join(root, 'raw')) };
  const upstreams = [new WebSocketServer({ port: 0 }), new WebSocketServer({ port: 0 })];
  await Promise.all(upstreams.map(server => once(server, 'listening')));
  const value = { enabled: true, bot_id: '123', groups: ['456', '457'], private_users: ['789'], business_commands: [],
    business_ws_url: `ws://127.0.0.1:${upstreams[0].address().port}` };
  const config = { snapshot: () => value, policy: () => value };
  const router = createRouter({ config: value, onebotToken: 'site', astrbotToken: 'chat', bridgeToken: 'bridge',
    policy: policy ?? (async () => config.snapshot()), chatUrl: `ws://127.0.0.1:${upstreams[1].address().port}`, actionTimeoutMs: 80 });
  router.server.listen(0, '127.0.0.1');
  await once(router.server, 'listening');
  const base = `http://127.0.0.1:${router.server.address().port}`;
  const gateway = new WebSocket(base.replace('http:', 'ws:') + '/onebot/ws', { headers: { Authorization: 'Bearer site', 'X-Self-ID': '123', 'X-Client-Role': 'Universal' } });
  const packets = [], spoken = [];
  gateway.on('message', raw => {
    const packet = JSON.parse(raw);
    packets.push(packet);
    if (acknowledge) return acknowledge(packet, gateway, packets.length);
    gateway.send(JSON.stringify({ echo: packet.echo, status: 'ok', retcode: 0, data: { message_id: 100 + packets.length } }));
  });
  await once(gateway, 'open');
  const sendPacket = groupPacketClient('bridge', base);
  const dependencies = () => ({ config, store: state.store, sendPacket, speech: async command => { spoken.push(command); return [audio]; } });
  const send = (args, context = {}, extra = {}) => sendGroupMessage(args, { input, id: 'source-turn', ...context }, { ...dependencies(), ...extra });
  t.after(async () => {
    gateway.terminate();
    await router.close();
    for (const server of upstreams) {
      for (const client of server.clients) client.terminate();
      await new Promise(resolve => server.close(resolve));
    }
    state.store.db.close();
    rmSync(root, { recursive: true, force: true });
  });
  return { root, state, value, config, base, router, packets, spoken, send, sendPacket, dependencies,
    restart: () => { state.store.db.close(); state.store = new Store(join(root, 'state.sqlite'), join(root, 'raw')); } };
}

test('text dispatch gets real receipts, preserves every character and never copies the private request to group history', async t => {
  const f = await fixture(t);
  f.state.store.observe('123:private:789', { event_id: 'msg:secret', sender: '789', text: '这是私聊内容，不发到群里' });
  const args = { group_id: '456', text: '😀'.repeat(1201) + '会议通知' };
  const result = await f.send(args);
  assert.equal(result.status, 'sent');
  assert.deepEqual(result.message_ids, ['101', '102']);
  assert.deepEqual(f.packets.map(p => p.action), ['send_group_msg', 'send_group_msg']);
  assert.ok(f.packets.every(p => p.params.group_id === 456));
  assert.equal(f.packets.map(p => p.params.message[0].data.text).join(''), args.text);
  assert.equal(f.state.store.history('123:group:456').map(row => row.text).join(''), args.text);
  assert.equal(f.state.store.history('123:group:457').length, 0);
  assert.deepEqual(await f.send(args, { call_id: 'another-model-call' }), result);
  await assert.rejects(f.send({ ...args, text: '修改后的内容' }), /提交过其他内容/u);
  f.restart();
  assert.deepEqual(await f.send(args), result);
  assert.equal(f.packets.length, 2);
});

test('voice uses target group defaults, supports explicit overrides and returns record receipts', async t => {
  const f = await fixture(t);
  f.value.reply_settings = { group_defaults: { mode: 'voice', voice: '御姐' }, private_users: { '789': { mode: 'text', voice: '曼波' } } };
  const result = await f.send({ group_id: '456', text: '大家晚上好' });
  assert.equal(result.status, 'sent');
  assert.equal(result.voice, '御姐');
  assert.equal(f.spoken[0].referenceId, replyVoice('御姐').referenceId);
  assert.deepEqual(f.packets[0].params.message, [{ type: 'record', data: { file: 'base64://' + audio.data } }]);
  assert.equal(f.state.store.history('123:group:456').filter(row => row.text === '大家晚上好').length, 1, '全部语音确认后才把授权朗读正文记入目标群');
  await f.send({ group_id: '456', text: '大家晚上好' });
  assert.equal(f.state.store.history('123:group:456').filter(row => row.text === '大家晚上好').length, 1, '重复工具调用不重复写正文');
  const text = await f.send({ group_id: '457', text: '这次发文字', mode: 'text' });
  assert.equal(text.mode, 'text');
  assert.equal(f.spoken.length, 1);
  await f.send({ group_id: '456', text: '换个声音', mode: 'voice', voice: '曼波' }, { id: 'next-turn' });
  assert.equal(f.spoken[1].referenceId, replyVoice('曼波').referenceId);
  await assert.rejects(f.send({ group_id: '456', text: '内容', voice: '不存在' }, { id: 'invalid' }), /unknown_voice/u);
});

test('ordinary group users, group admins and allowlisted users in groups cannot dispatch; current-group voice selection stays available', async t => {
  const f = await fixture(t);
  for (const sender of ['789', '999']) for (const role of ['member', 'admin', 'owner']) {
    const context = { input: { ...input, group: true, chat: '456', sender, role } };
    await assert.rejects(executeCapability({ name: 'send_group_message', args: { group_id: '457', text: '越权' } }, context,
      { config: f.config, sendGroupMessage: () => { throw new Error('不应到达发送器'); } }), /私聊/u);
    await executeCapability({ name: 'set_reply_mode', args: { mode: 'voice', voice: '御姐' } }, context, { config: f.config });
    assert.equal(context.replyVoice.voice, '御姐');
  }
  await assert.rejects(f.send({ group_id: '456', text: '越权' }, { input: { ...input, chat: '999', sender: '999' } }), /权限已关闭/u);
  await assert.rejects(f.send({ group_id: '999', text: '越权' }), /目标群/u);
  f.value.group_policies = [{ group_id: '456', disable_chat: true }];
  await assert.rejects(f.send({ group_id: '456', text: '越权' }), /目标群/u);
  assert.equal(f.packets.length, 0);
});

test('revocation during synthesis prevents send and failures cannot be retried by changing call id', async t => {
  const f = await fixture(t);
  const args = { group_id: '456', text: '大家好', mode: 'voice' };
  await assert.rejects(f.send(args, {}, { speech: async () => { f.value.private_users = []; return [audio]; } }), /权限已关闭/u);
  f.value.private_users = ['789'];
  assert.equal((await f.send(args, { call_id: 'retry' })).status, 'failed');
  assert.equal(f.packets.length, 0);
  assert.equal(f.spoken.length, 0);
  await assert.rejects(f.send({ ...args, text: 'token=sk-1234567890abcdefghijklmnopqrstuvwxyz' }, { id: 'sensitive' }), /密钥|口令/u);
});

test('partial text delivery records only acknowledged content and is not replayed after restart', async t => {
  const f = await fixture(t, (packet, gateway, count) => {
    if (count === 1) gateway.send(JSON.stringify({ echo: packet.echo, status: 'ok', retcode: 0, data: { message_id: '101' } }));
    if (count === 2) gateway.close();
  });
  const args = { group_id: '456', text: '甲'.repeat(1200) + '乙' };
  const result = await f.send(args);
  assert.equal(result.status, 'partial');
  assert.deepEqual(result.message_ids, ['101']);
  assert.equal(f.state.store.history('123:group:456').map(row => row.text).join(''), '甲'.repeat(1200));
  f.restart();
  const duplicate = await f.send(args);
  assert.equal(duplicate.status, 'partial');
  assert.deepEqual(duplicate.message_ids, ['101']);
  assert.equal(f.packets.length, 2);
});

test('missing receipt times out as unconfirmed and a crash in sending is never replayed', async t => {
  const f = await fixture(t, () => {});
  const args = { group_id: '456', text: '结果未知' };
  assert.equal((await f.send(args)).status, 'unconfirmed');
  assert.equal((await f.send(args)).status, 'unconfirmed');
  assert.equal(f.packets.length, 1);
  assert.equal(f.state.store.history('123:group:456').length, 0);
  const id = opaqueKey('crashed:send-group:456');
  f.state.store.beginGroupDispatch(id, '123:group:456', JSON.stringify(['456', args.text, null, null]), 'text', '默认', 'crashed');
  f.state.store.finish(id, 'completed', args.text);
  f.state.store.claimDelivery(id);
  f.restart();
  assert.equal((await f.send(args, { id: 'crashed' })).status, 'unconfirmed');
  assert.equal(f.packets.length, 1);
});

test('partial voice delivery keeps acknowledged segment markers without claiming the entire transcript was spoken', async t => {
  const f = await fixture(t, (packet, gateway, count) => {
    if (count === 1) gateway.send(JSON.stringify({ echo: packet.echo, status: 'ok', retcode: 0, data: { message_id: '101' } }));
    if (count === 2) gateway.close();
  });
  const result = await f.send({ group_id: '456', text: '完整通知正文', mode: 'voice' }, {}, { speech: async () => [audio, audio] });
  assert.equal(result.status, 'partial');
  assert.deepEqual(f.state.store.history('123:group:456').map(row => row.text), ['[已发送默认语音]']);
});

test('router accepts only authenticated text/record sends and rechecks current permission', async t => {
  const f = await fixture(t);
  const body = { bot: '123', sender: '789', group_id: '456', segment: { type: 'text', data: { text: '你好' } } };
  const post = (value, token = 'bridge') => fetch(f.base + '/send-group', { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-bridge-token': token }, body: JSON.stringify(value) });
  assert.equal((await post(body, 'wrong')).status, 401);
  assert.equal((await post({ ...body, segment: { type: 'at', data: { qq: 'all' } } })).status, 400);
  assert.equal((await post({ ...body, segment: { type: 'record', data: { file: 'https://example.com/audio' } } })).status, 400);
  f.value.private_users = [];
  assert.equal((await post(body)).status, 403);
  f.value.private_users = ['789'];
  f.value.chat_disabled = true;
  assert.equal((await post(body)).status, 403);
  assert.equal(f.packets.length, 0);
});

test('client cancellation while router waits for policy prevents a late QQ send', async t => {
  const waiting = Promise.withResolvers(), permission = Promise.withResolvers(), finished = Promise.withResolvers();
  const f = await fixture(t, undefined, { policy: async () => {
    waiting.resolve();
    await permission.promise;
    return f.value;
  } });
  const disconnected = Promise.withResolvers();
  f.router.server.once('request', (req, res) => res.once('close', () => disconnected.resolve()));
  const controller = new AbortController();
  const sending = f.send({ group_id: '456', text: '取消后不发送' }, { signal: controller.signal });
  await waiting.promise;
  controller.abort();
  assert.equal((await sending).status, 'unconfirmed');
  await disconnected.promise;
  // 再发一次合法请求，确保前一条权限请求已恢复并经过路由器事件循环。
  permission.resolve();
  f.router.server.once('request', () => finished.resolve());
  await fetch(f.base + '/health');
  await finished.promise;
  assert.equal(f.packets.length, 0);
  assert.equal(f.state.store.history('123:group:456').length, 0);
  assert.equal((await f.send({ group_id: '456', text: '取消后不发送' })).status, 'unconfirmed');
});

test('invalid QQ receipts do not become success and target revocation stops later packets', async t => {
  await t.test('zero or failed receipt', async t => {
    const f = await fixture(t, (packet, gateway) => gateway.send(JSON.stringify({ echo: packet.echo, status: 'ok', retcode: 0, data: { message_id: 0 } })));
    const result = await f.send({ group_id: '456', text: '无有效回执' });
    assert.equal(result.status, 'unconfirmed');
    assert.deepEqual(result.message_ids, []);
    assert.equal(f.state.store.history('123:group:456').length, 0);
  });
  await t.test('target disabled after synthesis', async t => {
    const f = await fixture(t);
    await assert.rejects(f.send({ group_id: '456', text: '你好', mode: 'voice' }, {}, { speech: async () => {
      f.value.group_policies = [{ group_id: '456', disable_chat: true }];
      return [audio];
    } }), /目标群/u);
    assert.equal(f.packets.length, 0);
  });
  await t.test('target disabled after first receipt', async t => {
    const f = await fixture(t);
    const result = await f.send({ group_id: '456', text: '甲'.repeat(1200) + '乙' }, {}, { sendPacket: async (body, options) => {
      const receipt = await f.sendPacket(body, options);
      f.value.group_policies = [{ group_id: '456', disable_chat: true }];
      return receipt;
    } });
    assert.equal(result.status, 'partial');
    assert.deepEqual(result.message_ids, ['101']);
    assert.equal(f.packets.length, 1);
  });
});

test('the actual bridge tool path waits for QQ receipt before giving the model its result', async t => {
  const f = await fixture(t);
  Object.assign(f.value, { model: 'test', model_api_key: 'key', context_window_tokens: 16384, max_output_tokens: 512,
    daily_chat_limit: 100, total_daily_chat_limit: 100, cooldown_seconds: 0 });
  const results = [];
  const bridge = createBridge({ config: f.config, token: 'bridge', store: f.state.store, root: f.root, sendPacket: f.sendPacket,
    makeHarness: async options => ({ start: async () => {}, close: async () => {}, session: () => ({ id: 'test-session', run: async () => {
      const response = await fetch(options.env.QQBOT_TOOL_URL, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-tool-token': options.env.QQBOT_TOOL_TOKEN },
        body: JSON.stringify({ session: 'test-session', call_id: '1', name: 'send_group_message', args: { group_id: '456', text: '正式通知' } }) });
      results.push(await response.json());
      return { finalResponse: '已发送，QQ 消息 ID：' + results[0].message_ids[0] };
    } }) }) });
  bridge.server.listen(0, '127.0.0.1');
  await once(bridge.server, 'listening');
  t.after(() => bridge.close());
  const response = await fetch(`http://127.0.0.1:${bridge.server.address().port}/turn`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-bridge-token': 'bridge' }, body: JSON.stringify(input) });
  const value = await response.json();
  assert.equal(response.status, 200);
  assert.equal(results[0].status, 'sent');
  assert.deepEqual(results[0].message_ids, ['101']);
  assert.match(value.text, /QQ 消息 ID：101/u);
});

test('a failed private summary reports persisted dispatch results instead of suggesting blind resubmission', async t => {
  for (const failure of ['model', 'private_voice']) await t.test(failure, async t => {
    const f = await fixture(t);
    Object.assign(f.value, { model: 'test', model_api_key: 'key', context_window_tokens: 16384, max_output_tokens: 512,
      daily_chat_limit: 100, total_daily_chat_limit: 100, cooldown_seconds: 0,
      reply_settings: { private_users: { '789': { mode: failure === 'private_voice' ? 'voice' : 'text' } } } });
    let calls = 0;
    const bridge = createBridge({ config: f.config, token: 'bridge', store: f.state.store, root: f.root, sendPacket: f.sendPacket,
      speech: async () => { throw new Error('voice_request_failed'); },
      makeHarness: async options => ({ start: async () => {}, close: async () => {}, session: () => ({ id: 'test-session', run: async () => {
        calls++;
        const response = await fetch(options.env.QQBOT_TOOL_URL, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-tool-token': options.env.QQBOT_TOOL_TOKEN },
          body: JSON.stringify({ session: 'test-session', call_id: '1', name: 'send_group_message', args: { group_id: '456', text: '正式通知', mode: 'text' } }) });
        assert.equal((await response.json()).status, 'sent');
        if (failure === 'model') throw new Error('model_request_failed');
        return { finalResponse: '已经发送' };
      } }) }) });
    bridge.server.listen(0, '127.0.0.1');
    await once(bridge.server, 'listening');
    t.after(() => bridge.close());
    const post = async () => {
      const response = await fetch(`http://127.0.0.1:${bridge.server.address().port}/turn`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-bridge-token': 'bridge' }, body: JSON.stringify(input) });
      assert.equal(response.status, 200);
      return response.json();
    };
    const result = await post();
    assert.match(result.text, /私聊回复未完成/u);
    assert.match(result.text, /群 456：已全部发送；QQ 消息 ID：101/u);
    assert.equal((await post()).text, result.text);
    // 还原进程在源回合完成前退出的状态，重放也只读持久回执。
    f.state.store.finish(result.id, 'interrupted');
    assert.equal((await post()).text, result.text);
    assert.equal(calls, 1);
    assert.equal(f.packets.length, 1);
  });
});

test('the total deadline preserves dispatch receipts in errors and private status queries', async t => {
  const f = await fixture(t);
  Object.assign(f.value, { model: 'test', model_api_key: 'key', context_window_tokens: 16384, max_output_tokens: 512,
    daily_chat_limit: 100, total_daily_chat_limit: 100, cooldown_seconds: 0, private_users: ['789', '888'] });
  const bridge = createBridge({ config: f.config, token: 'bridge', store: f.state.store, root: f.root,
    sendPacket: f.sendPacket, turnTimeoutMs: 500,
    makeHarness: async options => ({ start: async () => {}, close: async () => {}, session: () => ({ id: 'test-session', run: async () => {
      const response = await fetch(options.env.QQBOT_TOOL_URL, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-tool-token': options.env.QQBOT_TOOL_TOKEN },
        body: JSON.stringify({ session: 'test-session', call_id: '1', name: 'send_group_message', args: { group_id: '456', text: '已发出的通知' } }) });
      assert.equal((await response.json()).status, 'sent');
      return new Promise(() => {});
    } }) }) });
  bridge.server.listen(0, '127.0.0.1');
  await once(bridge.server, 'listening');
  t.after(() => bridge.close());
  const post = async (path, body, token = 'bridge') => {
    const response = await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-bridge-token': token }, body: JSON.stringify(body) });
    return { status: response.status, ...await response.json() };
  };
  const result = await post('/turn', input);
  assert.equal(result.error, 'turn_expired');
  assert.match(result.message, /群 456：已全部发送；QQ 消息 ID：101/u);
  assert.doesNotMatch(result.message, /请重新发送/u);
  assert.deepEqual(await post('/dispatch/status', input), { status: 200, found: true, message: result.message });
  assert.equal((await post('/dispatch/status', input, 'wrong')).status, 401);
  assert.equal((await post('/dispatch/status', { ...input, group: true, chat: '456' })).error, 'scope_denied');
  assert.equal((await post('/dispatch/status', { ...input, sender: '888', chat: '789' })).error, 'scope_denied');
  assert.equal((await post('/dispatch/status', { ...input, sender: '888', chat: '888' })).found, false);
  assert.equal((await post('/dispatch/status', { ...input, message_id: '2' })).found, false);
  assert.equal(f.packets.length, 1);
});
