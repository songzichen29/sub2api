import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { conversationScope, prepareMemory, retrieveMemory, sensitiveText, migrateMemory } from '../src/memory.mjs';

function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'qqbot-memory-'));
  const store = new Store(join(dir, 'db'), join(dir, 'raw'));
  t.after(() => { store.db.close(); rmSync(dir, { recursive: true, force: true }); });
  return store;
}
const member = { bot: '123', group: true, chat: '456', sender: '789', message_id: '1' };

test('a group shares all members and retains more than 100 messages across restart', t => {
  const s = fixture(t), scope = conversationScope(member);
  assert.equal(scope, conversationScope({ ...member, sender: '888' }));
  assert.notEqual(scope, conversationScope({ ...member, chat: '999' }));
  assert.notEqual(scope, conversationScope({ ...member, group: false, chat: '789' }));
  for (let i = 0; i < 150; i++) s.observe(scope, { event_id: String(i), sender: i % 2 ? '789' : '888', text: '公开聊天' + i });
  const other = new Store(join(s.rawDirectory, '..', 'db'), s.rawDirectory);
  assert.equal(other.history(scope).length, 150);
  assert.equal(other.history(scope)[0].text, '公开聊天0');
  other.db.close();
});

test('capacity summaries preserve originals, retrieve old facts, and isolate reset scopes', async t => {
  const s = fixture(t), scope = conversationScope(member);
  s.observe(scope, { event_id: 'first', sender: '789', text: '小李约定周五去紫金山野餐' });
  for (let i = 0; i < 55; i++) s.observe(scope, { event_id: String(i), sender: '888', text: '讨论后续活动安排'.repeat(12) });
  let calls = 0;
  const memory = await prepareMemory(s, scope, s.latest(scope), 4096, async () => { calls++; return 'QQ789：约定周五去紫金山野餐。'; });
  assert.ok(calls > 0);
  assert.equal(s.history(scope).length, 56);
  assert.ok(memory.messages.length > 0);
  assert.equal(retrieveMemory(s, scope, '紫金山野餐是什么时候', memory.summary_through, 2000)[0].sender, '789');
  assert.deepEqual(retrieveMemory(s, '123:group:999', '紫金山野餐', memory.summary_through, 2000), []);
  s.clear(scope);
  assert.deepEqual(s.history(scope), []);
  assert.equal(s.db.prepare('SELECT COUNT(*) AS n FROM history WHERE scope=?').get(scope).n, 56);
  assert.deepEqual(retrieveMemory(s, scope, '紫金山野餐', memory.summary_through, 2000), []);
});

test('a failed summary of a long message never advances its cursor', async t => {
  const s = fixture(t), scope = conversationScope(member);
  s.observe(scope, { event_id: 'long', sender: '789', text: '长消息'.repeat(3000) });
  s.observe(scope, { event_id: 'now', sender: '888', text: '继续' });
  let calls = 0;
  await assert.rejects(prepareMemory(s, scope, s.latest(scope), 4096, async () => { if (++calls === 2) throw new Error('upstream'); return '短摘要'; }), /upstream/);
  assert.equal(s.summary(scope).through_seq, 0);
  assert.equal(s.history(scope)[0].text.length, 9000);
});

test('credentials are excluded; legacy private and unconfirmed assistant memory never enters a group', t => {
  for (const text of ['绑定 123456', '确认 123456', '/绑定 a', '密码：123', 'test@example.com', 'sk-test12345678']) assert.equal(sensitiveText(text), true, text);
  const s = fixture(t);
  s.remember('123:group:456:789', { user: '我喜欢爬山', assistant: 'unconfirmed reply' });
  s.remember('123:private:789:789', { user: '私聊秘密', assistant: 'private reply' });
  s.remember('123:group:456:789', { user: '绑定 123456', assistant: 'unsafe' });
  migrateMemory(s, { bot_id: '123', groups: ['456'], private_users: ['789'] });
  assert.deepEqual(s.history('123:group:456').map(r => r.text), ['我喜欢爬山']);
  assert.deepEqual(s.history('123:private:789').map(r => r.text), ['私聊秘密']);
  migrateMemory(s, { bot_id: '123', groups: ['456'], private_users: ['789'] });
  assert.equal(s.history('123:group:456').length, 1);
});
