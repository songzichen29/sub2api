import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { createBridge } from '../src/bridge.mjs';
import { synthesizeVoice } from '../src/voice.mjs';
import { opaqueKey, scopeKey } from '../src/policy.mjs';

async function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'qq-voice-'));
  const store = new Store(join(root, 'db'), join(root, 'raw'));
  const config = { enabled: true, bot_id: '123', groups: ['456'], private_users: ['789'],
    business_commands: ['绑定'], total_daily_chat_limit: 100, daily_chat_limit: 10, cooldown_seconds: 0,
    memes_enabled: false, group_policies: [{ group_id: '456', disable_images: true }] };
  const requests = [];
  const state = { status: 200, body: Buffer.from('ID3\x04\x00\x00\x00\x00\x00\x00', 'binary') };
  const upstream = http.createServer(async (req, res) => {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    requests.push(JSON.parse(Buffer.concat(chunks)));
    res.writeHead(state.status, { 'Content-Type': 'audio/mpeg' }); res.end(state.body);
  });
  upstream.listen(0, '127.0.0.1'); await once(upstream, 'listening');
  const bridge = createBridge({ config: { snapshot: () => config }, token: 'bridge-test', store, root,
    makeHarness: () => { throw new Error('语音命令不得调用聊天模型'); },
    speech: (command, options) => synthesizeVoice(command, {
      ...options, fetcher: (url, init) => fetch(`http://127.0.0.1:${upstream.address().port}`, init)
    }) });
  bridge.server.listen(0, '127.0.0.1'); await once(bridge.server, 'listening');
  t.after(async () => {
    await bridge.close();
    await new Promise(resolve => upstream.close(resolve));
    store.db.close(); rmSync(root, { recursive: true, force: true });
  });
  const post = async (path, body) => {
    const response = await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`, {
      method: 'POST', headers: { 'x-bridge-token': 'bridge-test', 'Content-Type': 'application/json' }, body: JSON.stringify(body)
    });
    return { status: response.status, ...await response.json() };
  };
  const input = { bot: '123', group: true, chat: '456', sender: '789', message_id: '1', text: '御姐语音合成 ' + '你好'.repeat(600) };
  return { store, config, requests, state, post, input };
}

test('voice turns use a local HTTP fixture, keep every segment, deduplicate and commit audio history only after receipts', async t => {
  const { store, requests, post, input } = await fixture(t);
  const result = await post('/turn', input);
  assert.equal(result.status, 200);
  assert.equal(result.text, ''); assert.equal(result.media.length, 3);
  assert.equal(requests.map(body => body.text).join(''), '你好'.repeat(600));
  assert.equal(store.history('123:group:456').filter(row => row.role === 'assistant').length, 0);
  const duplicate = await post('/turn', input);
  assert.equal(duplicate.duplicate, true); assert.deepEqual(duplicate.media, result.media);
  assert.equal(requests.length, 3);
  assert.equal((await post('/delivery/claim', { id: result.id })).claimed, true, '图片和表情包开关不禁止语音');
  assert.equal((await post('/delivery/claim', { id: result.id })).claimed, false);
  for (const message_id of ['101', '102', '103']) {
    assert.equal((await post('/delivery/receipt', { id: result.id, message_ids: [message_id] })).status, 200);
  }
  assert.equal((await post('/delivery/finish', { id: result.id, status: 'sent', message_ids: ['101', '102', '103'] })).status, 200);
  const replies = store.history('123:group:456').filter(row => row.role === 'assistant');
  assert.equal(replies.length, 1); assert.match(replies[0].text, /已发送语音/u); assert.doesNotMatch(replies[0].text, /表情包/u);
});

test('voice commands read capability questions literally instead of returning the capability menu', async t => {
  const { requests, post, input } = await fixture(t);
  const result = await post('/turn', { ...input, text: '御姐语音合成 你有什么工具' });
  assert.equal(result.status, 200);
  assert.equal(result.text, ''); assert.equal(result.media.length, 1);
  assert.deepEqual(requests.map(body => body.text), ['你有什么工具']);
});

test('voice respects private users, scope, sensitive-data checks and current chat quotas', async t => {
  const { config, requests, post, input } = await fixture(t);
  const privateTurn = { ...input, group: false, chat: '789', text: '/语音合成 你好' };
  assert.equal((await post('/turn', privateTurn)).status, 200);
  assert.equal((await post('/turn', { ...privateTurn, chat: '888', sender: '888' })).error, 'scope_denied');
  assert.equal((await post('/turn', { ...input, chat: '999' })).error, 'scope_denied');
  assert.equal(requests.length, 1);
  const sensitive = await post('/turn', { ...input, text: '语音合成 API Key: sk-1234567890abcdefghijklmnop' });
  assert.match(sensitive.text, /请勿/u); assert.equal(requests.length, 1);
  config.total_daily_chat_limit = 1;
  assert.equal((await post('/turn', { ...input, message_id: '2' })).error, 'rate_limited');
  assert.equal(requests.length, 1);
});

test('permissions are checked again before voice delivery and partial delivery is never replayed', async t => {
  const { config, post, input } = await fixture(t);
  const result = await post('/turn', input);
  assert.equal(result.status, 200);
  config.group_policies[0].disable_chat = true;
  assert.equal((await post('/delivery/claim', { id: result.id })).claimed, false);
  config.group_policies[0].disable_chat = false;
  const next = await post('/turn', { ...input, message_id: '2' });
  assert.equal((await post('/delivery/claim', { id: next.id })).claimed, true);
  assert.equal((await post('/delivery/finish', { id: next.id, status: 'partial', message_ids: ['101'] })).status, 200);
  assert.equal((await post('/delivery/claim', { id: next.id })).claimed, false);
});

test('failed generation stays failed with no outgoing audio or automatic re-execution', async t => {
  const { store, requests, state, post, input } = await fixture(t);
  state.body = Buffer.from('{"error":"invalid reference"}');
  const failed = await post('/turn', input);
  assert.equal(failed.error, 'voice_response_invalid');
  const id = opaqueKey(scopeKey(input) + ':' + input.message_id);
  assert.equal(store.turn(id).status, 'failed'); assert.deepEqual(store.media(id), []);
  assert.equal((await post('/turn', input)).error, 'previous_execution_unconfirmed');
  assert.equal((await post('/delivery/claim', { id })).claimed, undefined);
  assert.equal(requests.length, 1);
});
