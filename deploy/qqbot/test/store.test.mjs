import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
test('scope isolation, uncertain execution and delivery survive restart', () => {
  const dir = mkdtempSync(join(tmpdir(), 'qqbot-'));
  let s = new Store(join(dir, 'db'), join(dir, 'raw'));
  s.remember('private:a', { user: 'private secret' });
  assert.deepEqual(s.recent('group:a'), []);
  s.begin('one', 'private:a'); s.finish('one', 'completed', 'hello');
  s.begin('interrupted', 'private:a');
  assert.equal(s.claimDelivery('one'), true);
  s.db.close();
  s = new Store(join(dir, 'db'), join(dir, 'raw'));
  assert.equal(s.turn('interrupted').status, 'interrupted');
  assert.equal(s.claimDelivery('one'), false);
  s.setSession('private:a', 'saved'); s.clear('private:a');
  assert.equal(s.session('private:a'), undefined);
  assert.deepEqual(s.recent('private:a'), []);
  s.db.close(); rmSync(dir, { recursive: true });
});

test('daily quotas and failed multi-scope reservations survive restart atomically', () => {
  const dir = mkdtempSync(join(tmpdir(), 'qqbot-'));
  let s = new Store(join(dir, 'db'), join(dir, 'raw'));
  s.reserve([{ key: 'all', limit: 3 }, { key: 'member', limit: 1 }]);
  assert.throws(() => s.reserve([{ key: 'all', limit: 3 }, { key: 'member', limit: 1 }]), /rate_limited/);
  assert.equal(s.db.prepare('SELECT used FROM meters WHERE key=?').get('all').used, 1);
  s.db.close(); s = new Store(join(dir, 'db'), join(dir, 'raw'));
  assert.throws(() => s.reserve([{ key: 'member', limit: 1 }]), /rate_limited/);
  s.db.close(); rmSync(dir, { recursive: true });
});

test('restart drops model sessions with uncertain delivery and retains confirmed sessions', () => {
  const dir = mkdtempSync(join(tmpdir(), 'qqbot-crash-'));
  let s = new Store(join(dir, 'db'), join(dir, 'raw'));
  for (const id of ['confirmed', 'uncertain']) {
    s.begin(id, id); s.finish(id, 'completed', 'reply');
    s.setConversation(id, 'session-' + id, 1, 500);
    assert.equal(s.claimDelivery(id), true);
  }
  s.finishDelivery('confirmed', 'sent');
  assert.throws(() => s.finishDelivery('unknown', 'sent'), /invalid_delivery_transition/);
  s.db.close();
  s = new Store(join(dir, 'db'), join(dir, 'raw'));
  assert.equal(s.conversation('uncertain'), undefined);
  assert.equal(s.conversation('confirmed').session_id, 'session-confirmed');
  assert.throws(() => s.finishDelivery('confirmed', 'unconfirmed'), /invalid_delivery_transition/);
  s.db.close(); rmSync(dir, { recursive: true });
});
