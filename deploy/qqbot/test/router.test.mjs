import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { setTimeout as delay } from 'node:timers/promises';
import { WebSocket, WebSocketServer } from 'ws';
import { createRouter } from '../src/router.mjs';

test('isolated OneBot routing and colliding API echoes retain one destination', { timeout: 12000 }, async () => {
  const business = new WebSocketServer({ port: 0 }), chat = new WebSocketServer({ port: 0 });
  await Promise.all([once(business, 'listening'), once(chat, 'listening')]);
  const config = { bot_id: '123', groups: ['456'], private_users: ['789'], business_commands: ['签到'], business_ws_url: `ws://127.0.0.1:${business.address().port}` };
  const router = createRouter({ config, onebotToken: 'site', astrbotToken: 'chat', chatUrl: `ws://127.0.0.1:${chat.address().port}` });
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
    for (let n = 0; n < 100 && frames.flat().filter(e => e.post_type === 'message').length < 2; n++) await delay(10);
    assert.deepEqual(frames[0].filter(e => e.post_type === 'message').map(e => e.message_id), [1]);
    assert.deepEqual(frames[1].filter(e => e.post_type === 'message').map(e => e.message_id), [2]);
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
