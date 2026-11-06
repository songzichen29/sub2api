import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { nextProactive, canDeliverProactive } from '../src/proactive.mjs';
import { beijingTime } from '../src/time.mjs';
import { quotaLimits } from '../src/quota.mjs';
import { createBridge } from '../src/bridge.mjs';

const noon = Date.parse('2026-11-06T12:00:05+08:00');
const settings = () => ({ enabled: true, bot_id: '123', groups: ['456', '777'], private_users: [], owners: [], business_commands: ['余额'],
  proactive_enabled: true, proactive_groups: ['456'], proactive_times: ['12:00', '18:00'], proactive_idle_minutes: 10,
  total_daily_chat_limit: 500, daily_chat_limit: 0, cooldown_seconds: 0, ambient_daily_calls: 100,
  ambient_daily_replies: 100, ambient_hourly_replies: 10, model: 'test', context_window_tokens: 16384, max_output_tokens: 128 });
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'qqbot-moments-'));
  const store = new Store(join(root, 'db'), join(root, 'raw'));
  t.after(() => { store.db.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, store };
}

test('Beijing time, midnight, and already-local timestamps are never converted twice', () => {
  assert.equal(beijingTime('2026-11-05T18:00:00Z'), '2026-11-06T02:00:00+08:00');
  assert.equal(beijingTime('2026-11-06T02:00:00+08:00'), '2026-11-06T02:00:00+08:00');
  assert.equal(beijingTime('2026-11-05T16:00:00Z'), '2026-11-06T00:00:00+08:00');
});

test('moments follow Beijing calendar, survive restarts, and never replay missed slots', t => {
  const { store } = fixture(t), c = settings();
  assert.equal(nextProactive(c, store, noon - 6000, noon - 3600000), undefined);
  const slot = nextProactive(c, store, noon, noon - 3600000);
  assert.equal(slot.slot, '12:00'); assert.equal(slot.chat, '456');
  assert.equal(nextProactive(c, store, noon + 15000, noon), undefined);
  assert.equal(nextProactive({ ...c, proactive_groups: ['777'] }, store, noon + 240000, noon + 120000), undefined, 'a restart after the moment does not backfill');
  const midnight = Date.parse('2026-11-07T00:00:01+08:00');
  const tomorrow = nextProactive({ ...c, proactive_times: ['00:00'] }, store, midnight, midnight - 120000);
  assert.equal(beijingTime(tomorrow.at), '2026-11-07T00:00:00+08:00');
});

test('live group activity, disabled policies and incoming messages cancel a moment', t => {
  const { store } = fixture(t), c = settings(), scope = '123:group:456';
  store.noteActivity(scope, 'human-1', noon - 1000);
  assert.equal(nextProactive(c, store, noon, noon - 3600000), undefined);
  assert.equal(nextProactive(c, store, noon + 60000, noon - 3600000), undefined, 'a busy slot is consumed, not retried later');
  for (const policy of [{ disable_notifications: true }, { disable_chat: true }]) {
    assert.equal(nextProactive({ ...c, proactive_groups: ['777'], group_policies: [{ group_id: '777', ...policy }] }, store, noon, noon - 3600000), undefined);
  }
  const job = nextProactive({ ...c, proactive_groups: ['777'] }, store, noon, noon - 3600000);
  store.finishProactive(job.id, 'generated', 'turn-1');
  const saved = store.proactiveTurn('turn-1');
  assert.equal(canDeliverProactive({ ...c, proactive_groups: ['777'] }, store, saved, noon), true);
  store.noteActivity(job.scope, 'human-2', noon + 1);
  assert.equal(canDeliverProactive(c, store, saved, noon + 2), false);
  assert.equal(nextProactive({ ...c, proactive_groups: ['999'] }, store, noon, noon - 3600000), undefined, 'no new groups are enabled');
});

test('bridge generates a moment with shared local-time context, real receipt and no member impersonation', async t => {
  const { root, store } = fixture(t), c = settings();
  let now = noon - 60000;
  const model = [], options = [];
  store.observe('123:group:456', { event_id: 'old', sender: '789', text: '凌晨两点睡觉', at: '2026-11-05T18:00:00Z' });
  const bridge = createBridge({ config: { snapshot: () => c, policy: () => c }, token: 'test', store, root, clock: () => now,
    makeHarness: async o => { options.push(o); return { start: async () => {}, close: async () => {}, session: () => ({ id: 'moment-session', run: async blocks => { model.push(JSON.parse(blocks[0].text)); return { finalResponse: '午饭都吃了啥？' }; } }) }; } });
  bridge.server.listen(0, '127.0.0.1'); await once(bridge.server, 'listening');
  t.after(() => bridge.close());
  const post = async (path, body) => {
    const r = await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`, { method: 'POST', headers: { 'x-bridge-token': 'test' }, body: JSON.stringify(body) });
    return { status: r.status, body: await r.json() };
  };
  assert.equal((await post('/proactive/next', {})).body.skipped, 'no_due_slot');
  now = noon;
  const result = await post('/proactive/next', {});
  assert.equal(result.status, 200); assert.equal(result.body.chat, '456');
  assert.equal(model[0].timezone, 'Asia/Shanghai');
  assert.equal(model[0].time, '2026-11-06T12:00:05+08:00');
  assert.equal(model[0].conversation_memory.messages[0].at, '2026-11-06T02:00:00+08:00');
  assert.equal(model[0].verified_sender, '123'); assert.match(model[0].mode, /不是任何群成员/u);
  assert.match(options[0].env.DSH_SYSTEM_PROMPT, /不得再次加八小时/u);
  assert.equal(store.history('123:group:456').length, 1, 'no fabricated human request in memory');
  assert.equal(store.conversation('123:group:456'), undefined, 'a background generation does not own the human session');
  assert.equal((await post('/delivery/claim', { id: result.body.id })).body.claimed, true);
  assert.equal((await post('/delivery/finish', { id: result.body.id, status: 'sent', message_ids: [] })).status, 400);
  assert.equal(store.proactiveTurn(result.body.id).status, 'generated');
  assert.equal((await post('/delivery/finish', { id: result.body.id, status: 'sent', message_ids: ['-12345'] })).status, 200);
  assert.equal(store.proactiveTurn(result.body.id).status, 'sent');
  assert.equal(store.history('123:group:456').at(-1).role, 'assistant');
  assert.equal((await post('/proactive/next', {})).body.skipped, 'no_due_slot');
  now = Date.parse('2026-11-06T18:00:02+08:00');
  const second = (await post('/proactive/next', {})).body;
  await post('/observe', { self_id: 123, post_type: 'message', message_type: 'group', group_id: 456, user_id: 789, message_id: 'new', message: [{ type: 'text', data: { text: '余额' } }] });
  assert.equal((await post('/delivery/claim', { id: second.id })).body.claimed, false, 'even business messages cancel generated topics');
  const limits = quotaLimits({ chat: '456', sender: '123', group: true, proactive: true, ambient: true }, c, noon);
  assert.ok(!limits.calls.some(r => /member|scope/u.test(r.key)));
});
