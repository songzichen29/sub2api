import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { createBridge } from '../src/bridge.mjs';
import { actionReport } from '../src/action-results.mjs';

const input = { bot: '123', group: false, chat: '789', sender: '789', message_id: '1', text: '1025362951 去这群打个招呼' };
const update = { version: 'v1', updates: [{ scope: 'group', group_ids: ['1025362951'], mode: 'voice', voice: '御姐', prompt: '只保存到后端的人设' }] };

async function fixture(t, run, dependencies = {}) {
  const root = mkdtempSync(join(tmpdir(), 'qq-action-result-'));
  const store = new Store(join(root, 'state.sqlite'), join(root, 'raw'));
  const value = { enabled: true, bot_id: '123', groups: ['1025362951'], private_users: ['789'],
    model: 'test', model_api_key: 'key', context_window_tokens: 16384, max_output_tokens: 512,
    daily_chat_limit: 1000, total_daily_chat_limit: 1000, cooldown_seconds: 0,
    reply_settings: { private_users: { '789': { mode: 'voice' } } } };
  const config = { snapshot: () => value, policy: () => value, refresh: async () => {} };
  const packets = [], spoken = [];
  let runs = 0;
  const bridge = createBridge({ config, token: 'test', store, root,
    speech: async command => { spoken.push(command); return [{ mimeType: 'audio/mpeg', data: 'SUQzBAAAAAAAAA==' }]; },
    sendPacket: async packet => { packets.push(packet); return String(packets.length); },
    siteTool: async body => body.tool === 'list_reply_groups' ? { groups: [{ group_id: '1025362951' }] } : { status: 'saved' },
    makeHarness: async options => ({ start: async () => {}, close: async () => {}, session: () => ({ id: 'session', run: async () => {
      runs++;
      let calls = 0;
      const tool = async (name, args, callId = String(++calls)) => {
        const response = await fetch(options.env.QQBOT_TOOL_URL, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-tool-token': options.env.QQBOT_TOOL_TOKEN },
          body: JSON.stringify({ session: 'session', call_id: callId, name, args }) });
        return { statusCode: response.status, ...await response.json() };
      };
      return { finalResponse: await run(tool) };
    } }) }), ...dependencies });
  bridge.server.listen(0, '127.0.0.1');
  await once(bridge.server, 'listening');
  const post = async (path = '/turn', body = input) => {
    const response = await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'x-bridge-token': 'test' }, body: JSON.stringify(body) });
    return { statusCode: response.status, ...await response.json() };
  };
  t.after(async () => { await bridge.close(); store.db.close(); rmSync(root, { recursive: true, force: true }); });
  return { root, store, config, value, post, packets, spoken, runs: () => runs };
}

test('rejected group then successful lookup cannot turn invented success into a message or voice', async t => {
  const f = await fixture(t, async tool => {
    const result = await tool('send_group_message', { group_id: '1025362958', text: '你好！新年好呀～', mode: 'text' });
    assert.equal(result.error, '目标群未开放或已关闭聊天');
    assert.equal((await tool('list_reply_groups', {})).groups[0].group_id, '1025362951');
    return '已发送语音：你好！新年好呀～\n发到1025362951群了～';
  });
  const result = await f.post();
  assert.equal(result.statusCode, 200);
  assert.match(result.text, /群 1025362958：未发送；目标群未开放/u);
  assert.doesNotMatch(result.text, /已发送|新年好/u);
  assert.equal(f.packets.length, 0);
  assert.equal(f.spoken.length, 0);
  assert.equal(f.store.conversation('123:private:789'), undefined);
  assert.equal((await f.post()).text, result.text);
  assert.equal(f.runs(), 1);
  const record = f.store.actions(result.id)[0];
  assert.equal(record.status, 'failed');
  assert.doesNotMatch(record.details.text || '', /新年/u);
  await f.post('/delivery/claim', { id: result.id });
  await f.post('/delivery/finish', { id: result.id, status: 'sent', message_ids: ['99'] });
  assert.equal(f.store.history('123:private:789').at(-1).text, result.text);
  assert.equal(f.store.conversation('123:private:789'), undefined, '不恢复含有模型假成功答复的会话');
});

test('actual text receipts override fake voice claims and a later rejected duplicate', async t => {
  const f = await fixture(t, async tool => {
    assert.equal((await tool('send_group_message', { group_id: '1025362951', text: '实际内容', mode: 'text' })).status, 'sent');
    assert.equal((await tool('send_group_message', { group_id: '1025362951', text: '不应再次发送', mode: 'voice' })).statusCode, 400);
    return '已发送语音';
  });
  const result = await f.post();
  assert.match(result.text, /群 1025362951：已全部发送文字；QQ 消息 ID：1/u);
  assert.doesNotMatch(result.text, /语音|未发送/u);
  assert.equal(f.packets.length, 1);
  assert.equal(f.spoken.length, 0);
});

test('write tools rejected by the existing call limit still override false success and deduplicate call IDs', async t => {
  for (const name of ['send_group_message', 'update_reply_settings']) await t.test(name, async t => {
    const f = await fixture(t, async tool => {
      for (let n = 0; n < 6; n++) assert.equal((await tool('list_reply_groups', {})).statusCode, 200);
      const args = name === 'send_group_message' ? { group_id: '1025362951', text: '不应发送' } : update;
      const result = await tool(name, args, 'limit-write');
      assert.equal(result.error, '本轮工具调用已达上限');
      assert.deepEqual(await tool(name, args, 'limit-write'), result);
      return '已发送语音，设置也已保存';
    });
    const result = await f.post();
    assert.match(result.text, /本轮工具调用已达上限/u);
    assert.match(result.text, name === 'send_group_message' ? /未发送/u : /未保存/u);
    assert.doesNotMatch(result.text, /已发送|已保存/u);
    assert.equal(f.store.actions(result.id).filter(row => row.name === name).length, 1);
    assert.equal(f.spoken.length, 0);
    assert.equal(f.packets.length, 0);
  });
});

test('a rejected wrong target can be corrected only by a real second send', async t => {
  const f = await fixture(t, async tool => {
    await tool('send_group_message', { group_id: '1025362958', text: '你好' });
    await tool('list_reply_groups', {});
    await tool('send_group_message', { group_id: '1025362951', text: '你好' });
    return '完成';
  });
  const result = await f.post();
  assert.match(result.text, /1025362958：未发送/u);
  assert.match(result.text, /1025362951：已全部发送文字/u);
  assert.equal(f.packets.length, 1);
});

test('real group voice receipts name its actual voice without speaking the private confirmation', async t => {
  const f = await fixture(t, async tool => {
    await tool('send_group_message', { group_id: '1025362951', text: '群里的语音', mode: 'voice', voice: '御姐' });
    return '已发文字';
  });
  const result = await f.post();
  assert.match(result.text, /已全部发送语音（御姐）；QQ 消息 ID：1/u);
  assert.equal(f.spoken.length, 1);
  assert.equal(f.spoken[0].text, '群里的语音');
  assert.equal(f.packets[0].segment.type, 'record');
  assert.deepEqual(result.media, []);
});

test('action confirmation preserves a separately prepared meme, including duplicate source turns', async t => {
  const f = await fixture(t, async tool => {
    await tool('update_reply_settings', update);
    assert.equal((await tool('send_meme', { keyword: '开心' })).status, 'prepared');
    return '设置好了，图片已发';
  });
  f.value.memes_enabled = true;
  const png = { mimeType: 'image/png', data: 'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg==' };
  f.store.collectMeme('123:private:789', png, '开心');
  const result = await f.post();
  assert.match(result.text, /回复设置已保存/u);
  assert.equal(result.media[0].data, png.data);
  assert.deepEqual((await f.post()).media, result.media);
  assert.equal(f.spoken.length, 0);
  assert.equal(f.runs(), 1);
});

test('combined queries and settings changes retain real catalogs and label the queried older settings', async t => {
  let voices;
  const f = await fixture(t, async tool => {
    voices = (await tool('list_voices', {})).voices;
    await tool('list_reply_groups', {});
    await tool('get_reply_settings', {});
    await tool('update_reply_settings', update);
    return '列表省略，已发送语音';
  }, { siteTool: async body => {
    if (body.tool === 'list_reply_groups') return { groups: [{ group_id: '1025362951', label: '工作群' }] };
    if (body.tool === 'get_reply_settings') return { settings: { group_defaults: { mode: 'text' }, groups: {}, private_users: { '789': { prompt: '不能复制的完整人设' } } }, base_prompt: '不能复制的站点知识' };
    return { status: 'saved' };
  } });
  const result = await f.post('/turn', { ...input, text: '有哪些音色和群？查询设置后把这个群改为御姐语音' });
  for (const voice of voices) assert.ok(result.text.includes(voice.name));
  assert.match(result.text, /1025362951（工作群）/u);
  assert.match(result.text, /本轮查询时的回复设置（以查询后保存结果为准）/u);
  assert.match(result.text, /群共享默认：回复方式：文字/u);
  assert.match(result.text, /回复设置已保存：群 1025362951（回复方式：语音；音色：御姐/u);
  assert.ok(result.text.indexOf('本轮查询时') < result.text.indexOf('回复设置已保存'));
  assert.doesNotMatch(result.text, /不能复制|已发送/u);
  assert.doesNotMatch(JSON.stringify(f.store.actions(result.id)), /不能复制/u);
  assert.equal(f.spoken.length, 0);
  assert.equal((await f.post('/turn', { ...input, text: '有哪些音色和群？查询设置后把这个群改为御姐语音' })).text, result.text);
});

test('query-only turns keep their existing answer and voice path', async t => {
  const f = await fixture(t, async tool => {
    await tool('list_voices', {});
    return '正常查询回答';
  });
  const result = await f.post('/turn', { ...input, text: '用语音告诉我有什么音色' });
  assert.equal(result.text, '');
  assert.equal(f.spoken[0].text, '正常查询回答');
  assert.equal(actionReport(f.store, result.id), undefined);
});

test('partial and unconfirmed sends override false success and never replay', async t => {
  for (const partial of [true, false]) await t.test(String(partial), async t => {
    let packets = 0;
    const f = await fixture(t, async tool => {
      const result = await tool('send_group_message', { group_id: '1025362951', text: '字'.repeat(1400), mode: 'text' });
      assert.equal(result.status, partial ? 'partial' : 'unconfirmed');
      return '全部发送成功，语音也发了';
    }, { sendPacket: async () => { packets++; if (partial && packets === 1) return '101'; throw new Error('gateway_timeout'); } });
    const result = await f.post();
    assert.match(result.text, /未确认/u);
    assert.doesNotMatch(result.text, /已全部|语音/u);
    assert.equal((await f.post()).text, result.text);
    assert.equal(packets, partial ? 2 : 1);
    assert.equal(f.spoken.length, 0);
  });
});

test('settings confirmations expose save, rejection and sync state without claiming a message was sent', async t => {
  for (const outcome of ['saved', 'rejected', 'unknown', 'sync_failed']) await t.test(outcome, async t => {
    let writes = 0;
    const f = await fixture(t, async tool => {
      await tool('update_reply_settings', update);
      await tool('get_reply_settings', {});
      await tool('update_reply_settings', update);
      return '行，群里以后用语音回复了～[已发送语音]';
    }, { siteTool: async body => {
      if (body.tool !== 'update_reply_settings') return { settings: { group_defaults: {}, groups: {}, private_users: {} } };
      writes++;
      if (outcome === 'rejected') throw Object.assign(new Error('版本冲突'), { status: 409 });
      if (outcome === 'unknown') throw new Error('connection_reset');
      return { status: 'saved' };
    } });
    f.config.refresh = async () => { if (outcome === 'sync_failed') throw new Error('network'); };
    const result = await f.post();
    assert.doesNotMatch(result.text, /已发送/u);
    if (outcome === 'saved' || outcome === 'sync_failed') assert.match(result.text, /回复设置已保存：群 1025362951（回复方式：语音；音色：御姐；人设已更新）/u);
    if (outcome === 'sync_failed') assert.match(result.text, /同步尚未确认/u);
    if (outcome === 'rejected') assert.match(result.text, /未保存；版本冲突/u);
    if (outcome === 'unknown') assert.match(result.text, /保存结果未确认/u);
    assert.equal(writes, 1);
    assert.equal(f.spoken.length, 0);
    assert.equal(f.packets.length, 0);
    assert.doesNotMatch(JSON.stringify(f.store.actions(result.id)), /只保存到后端的人设/u);
  });
});

test('rejection survives model errors, total timeout, status lookup and reopened storage', async t => {
  for (const timeout of [false, true]) await t.test(String(timeout), async t => {
    const f = await fixture(t, async tool => {
      await tool('send_group_message', { group_id: '1025362958', text: '不应发送' });
      if (timeout) return new Promise(() => {});
      throw new Error('model_failed');
    }, { turnTimeoutMs: 1000 });
    const result = await f.post();
    const report = result.text || result.message;
    assert.match(report, /1025362958：未发送/u);
    assert.doesNotMatch(report, /请重新发送/u);
    assert.equal((await f.post('/dispatch/status')).message, report);
    const id = f.store.db.prepare('SELECT id FROM turns').get().id;
    // 先关闭实际 bridge 的 SQLite，再从同一文件重开，模拟源回合中断后的恢复。
    f.store.finish(id, 'interrupted');
    f.store.db.close();
    f.store.db = new Store(join(f.root, 'state.sqlite'), join(f.root, 'raw')).db;
    assert.equal(actionReport(f.store, id), report);
    assert.equal((await f.post()).text, report);
    assert.equal(f.runs(), 1);
    assert.equal(f.spoken.length, 0);
    assert.equal(f.packets.length, 0);
  });
});

test('deadline reports persisted settings save stage rather than an invented final answer', async t => {
  for (const stage of ['saving', 'saved_sync_pending', 'saved']) await t.test(stage, async t => {
    let release;
    const pending = new Promise(resolve => { release = resolve; });
    const f = await fixture(t, async tool => {
      await tool('update_reply_settings', update);
      return new Promise(() => {});
    }, { turnTimeoutMs: 700, siteTool: async () => {
      if (stage === 'saving') await pending;
      return { status: 'saved' };
    } });
    f.config.refresh = async () => { if (stage === 'saved_sync_pending') await pending; };
    const result = await f.post();
    assert.equal(result.error, 'turn_expired');
    assert.match(result.message, stage === 'saving' ? /保存结果未确认/u : /回复设置已保存/u);
    if (stage === 'saved_sync_pending') assert.match(result.message, /同步尚未确认/u);
    assert.doesNotMatch(result.message, /已发送|请重新发送/u);
    const row = f.store.db.prepare('SELECT status FROM action_results').get();
    assert.equal(row.status, stage);
    release();
    // 等待测试中的外部调用返回，避免清理 SQLite 时仍有未完成写入。
    for (let n = 0; n < 100 && f.store.db.prepare('SELECT status FROM action_results').get().status !== 'saved'; n++) {
      await new Promise(resolve => setTimeout(resolve, 5));
    }
    assert.equal(f.store.db.prepare('SELECT status FROM action_results').get().status, 'saved');
    assert.match((await f.post('/dispatch/status')).message, /回复设置已保存/u);
    assert.equal(f.spoken.length, 0);
  });
});
