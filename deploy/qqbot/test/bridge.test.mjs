import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { createBridge } from '../src/bridge.mjs';
import { RuntimeConfig } from '../src/runtime-config.mjs';

test('shared bridge context, confirmed memory, restart recovery, configuration changes and scope checks', async t => {
  const dir = mkdtempSync(join(tmpdir(), 'qqbot-bridge-'));
  const store = new Store(join(dir, 'db'), join(dir, 'raw'));
  const c = { enabled: true, bot_id: '123', groups: ['456'], private_users: ['789'], owners: ['789'], business_commands: ['绑定', '余额'],
    model: 'test', model_base_url: 'https://example.invalid/v1', model_api_key: 'test-key', persona: 'persona1',
    total_daily_chat_limit: 500, daily_chat_limit: 100, cooldown_seconds: 0, context_window_tokens: 8192, max_output_tokens: 128 };
  const config = { snapshot: () => c, policy: () => ({ ...c, model_api_key: undefined }) };
  const calls = [], resumed = []; let n = 0, started = 0;
  const makeHarness = async () => ({ start: async () => { started++; }, close: async () => {},
    session: () => ({ id: 'session-' + ++n, run: async blocks => { calls.push(JSON.parse(blocks[0].text)); return { finalResponse: '回答' + n }; } }),
    resumeSession: async id => { resumed.push(id); return { id, run: async blocks => { calls.push(JSON.parse(blocks[0].text)); return { finalResponse: '恢复回答' }; } }; } });
  let bridge;
  const start = async () => { bridge = createBridge({ config, token: 'bridge-test', store, root: dir, makeHarness }); bridge.server.listen(0, '127.0.0.1'); await once(bridge.server, 'listening'); };
  await start();
  t.after(async () => { await bridge.close(); store.db.close(); rmSync(dir, { recursive: true, force: true }); });
  const post = async (path, body) => { const r = await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`, { method: 'POST', headers: { 'x-bridge-token': 'bridge-test', 'Content-Type': 'application/json' }, body: JSON.stringify(body) }); return { status: r.status, body: await r.json() }; };
  const observe = (id, sender, text, chat = '456') => post('/observe', { post_type: 'message', message_type: 'group', self_id: 123, group_id: chat, user_id: sender, message_id: id, message: [{ type: 'text', data: { text } }] });
  await observe('1', '888', '我约好周五爬山');
  await observe('2', '789', '我带水');
  assert.equal((await observe('secret', '789', '绑定 123456')).body.stored, false);
  assert.equal((await observe('other', '789', '别群的消息', '999')).body.stored, false);
  const input = { bot: '123', chat: '456', sender: '789', group: true, message_id: '3', text: '大家约好了什么' };
  const first = await post('/turn', input);
  assert.equal(first.status, 200);
  assert.deepEqual(calls[0].conversation_memory.messages.map(r => r.sender), ['888', '789', '789']);
  assert.equal(store.history('123:group:456').filter(r => r.role === 'assistant').length, 0);
  assert.equal((await post('/delivery/claim', { id: first.body.id })).body.claimed, true);
  await post('/delivery/finish', { id: first.body.id, status: 'sent' });
  await post('/delivery/finish', { id: first.body.id, status: 'sent' });
  assert.equal(store.history('123:group:456').filter(r => r.role === 'assistant').length, 1);
  await bridge.close(); await start();
  await post('/turn', { ...input, sender: '888', message_id: '4', text: '我带什么' });
  assert.equal(resumed[0], first.body.session);
  assert.ok(!JSON.stringify(calls[1].conversation_memory.messages).includes('我带水'), 'consumed history is not injected twice');
  c.persona = 'persona2';
  const third = await post('/turn', { ...input, message_id: '5', text: '换人格了吗' });
  assert.equal(third.status, 200);
  assert.equal(started, 3);
  assert.notEqual(third.body.session, first.body.session);
  assert.ok(JSON.stringify(calls[2]).includes('我约好周五爬山'), 'public memory survives persona change');
  assert.equal((await post('/turn', { ...input, chat: '999', message_id: '6' })).status, 400);
  c.enabled = false;
  assert.equal((await post('/turn', { ...input, message_id: '7' })).status, 400);
});

test('runtime policy follows the site, redacts credentials and rejects stale configuration', async () => {
  let version = 'a', reported;
  const runtime = new RuntimeConfig({ bot_id: '123', site_config_url: 'https://example.invalid/api/runtime/config' }, 'auth', {
    fetcher: async (url, options) => {
      assert.equal(options.headers.Authorization, 'Bearer auth');
      if (options.method === 'POST') { reported = JSON.parse(options.body); return { ok: true }; }
      return { ok: true, json: async () => ({ bot_id: '123', groups: ['456'], private_users: ['789'], revision: version.repeat(64), model_api_key: 'secret', persona: 'private prompt', model: version }) };
    }
  });
  await runtime.refresh(); assert.equal(runtime.snapshot().model, 'a');
  assert.equal(runtime.policy().model_api_key, undefined); assert.equal(runtime.policy().persona, undefined);
  version = 'b'; await runtime.refresh(); assert.equal(runtime.snapshot().model, 'b');
  assert.equal(reported.revision, 'b'.repeat(64));
  runtime.updated -= 46000; assert.throws(() => runtime.snapshot(), /configuration_not_synchronized/);
});
