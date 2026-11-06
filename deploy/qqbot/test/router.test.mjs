import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { setTimeout as delay } from 'node:timers/promises';
import { WebSocket, WebSocketServer } from 'ws';
import { createRouter } from '../src/router.mjs';

test('isolated OneBot routing and colliding API echoes retain one destination', { timeout: 12000 }, async () => {
  const business = new WebSocketServer({ port: 0 }), chat = new WebSocketServer({ port: 0 });
  await Promise.all([once(business, 'listening'), once(chat, 'listening')]);
  const config = { bot_id: '123', groups: ['456'], private_users: ['789'], business_commands: ['签到'], business_observation: true, business_ws_url: `ws://127.0.0.1:${business.address().port}` };
  const observed = [];
  const router = createRouter({ config, onebotToken: 'site', astrbotToken: 'chat', chatUrl: `ws://127.0.0.1:${chat.address().port}`, observe: async e => { observed.push(e.message_id); } });
  router.server.listen(0, '127.0.0.1'); await once(router.server, 'listening');
  const frames = [[], []], peers = [];
  for (const [i, upstream] of [business, chat].entries()) upstream.on('connection', ws => { peers[i] = ws; ws.on('message', raw => frames[i].push(JSON.parse(raw))); });
  await delay(30);
  assert.equal(peers.length, 0, 'staging must not occupy the existing business connection');
  const gateway = new WebSocket(`ws://127.0.0.1:${router.server.address().port}/onebot/ws`, { headers: { Authorization: 'Bearer site', 'X-Self-ID': '123', 'X-Client-Role': 'Universal' } });
  try {
    await once(gateway, 'open');
    for (let n = 0; n < 100 && peers.filter(Boolean).length !== 2; n++) await delay(10);
    assert.equal(peers.filter(Boolean).length, 2);
    const event = (id, text) => ({ self_id: 123, user_id: 789, group_id: 456, message_id: id, post_type: 'message', message_type: 'group', message: [{ type: 'text', data: { text } }] });
    gateway.send(JSON.stringify(event(1, '签到'))); gateway.send(JSON.stringify(event(2, '你好')));
    gateway.send('null'); gateway.send(JSON.stringify({ ...event(3, 'bad'), message: 'bad' }));
    gateway.send(JSON.stringify({ ...event(4, '给其他人的消息'), message: [{ type: 'at', data: { qq: '999' } }, ...event(4, '给其他人的消息').message] }));
    gateway.send(JSON.stringify({ ...event(5, '外群'), group_id: 999 }));
    gateway.send(JSON.stringify({ ...event(6, '机器人自身'), user_id: 123 }));
    gateway.send(JSON.stringify(event(7, '查下我的余额')));
    gateway.send(JSON.stringify(event(8, '我让你查下我的余额')));
    gateway.send(JSON.stringify(event(9, '余额是什么')));
    for (let n = 0; n < 100 && frames.flat().filter(e => e.post_type === 'message').length < 8; n++) await delay(10);
    const received = frames[0].filter(e => e.post_type === 'message');
    assert.deepEqual(received.filter(e => !e.sub2api_observe_only).map(e => e.message_id), [1, 7, 8], 'fixed commands and personal queries execute once without rewriting the original text');
    assert.equal(received.find(e => e.message_id === 7).message[0].data.text, '查下我的余额');
    assert.deepEqual(received.filter(e => e.sub2api_observe_only).map(e => e.message_id), [2, 4, 9], 'chat and mentions of others still reach moderation');
    assert.deepEqual(frames[1].filter(e => e.post_type === 'message').map(e => e.message_id), [2, 9]);
    assert.deepEqual(observed, [1, 2, 4, 7, 8, 9], 'all allowed human traffic suppresses stale proactive topics, including business commands');
    assert.equal(frames[1].find(e => e.message_id === 2).sub2api_observe_only, undefined);
    const largeImage = { ...event(10, ''), message: [{ type: 'image', data: { file: 'x'.repeat(2 * 1024 * 1024) } }] };
    gateway.send(JSON.stringify(largeImage));
    for (let n = 0; n < 100 && !frames[1].some(e => e.message_id === 10); n++) await delay(10);
    assert.equal(frames[1].find(e => e.message_id === 10)?.message[0].data.file.length, 2 * 1024 * 1024, 'OneBot image payloads larger than the old 2 MiB limit reach chat');
    const actions = [];
    gateway.on('message', raw => { const a = JSON.parse(raw); actions.push(a); gateway.send(JSON.stringify({ status: 'ok', retcode: 0, data: { origin: a.params.origin }, echo: a.echo })); });
    for (const [i, peer] of peers.entries()) peer.send(JSON.stringify({ action: 'get_status', params: { origin: i }, echo: 'identical-echo' }));
    for (let n = 0; n < 100 && !frames.every(f => f.some(e => e.echo === 'identical-echo')); n++) await delay(10);
    assert.equal(actions.length, 2); assert.notEqual(actions[0].echo, actions[1].echo);
    for (let i = 0; i < 2; i++) assert.deepEqual(frames[i].find(e => e.echo === 'identical-echo').data, { origin: i });
  } finally {
    gateway.terminate(); await router.close();
    for (const s of [business, chat]) { for (const ws of s.clients) ws.terminate(); await new Promise(resolve => s.close(resolve)); }
  }
});
