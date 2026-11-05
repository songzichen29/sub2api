import test from 'node:test';
import assert from 'node:assert/strict';
import { routeEvent, scopeKey, authorized } from '../src/policy.mjs';
const c = { bot_id: '123', groups: ['456'], private_users: ['789'], business_commands: ['签到', '余额', '绑定', '转账', '画图', '生图'] };
const event = text => ({ self_id: 123, post_type: 'message', message_type: 'group', group_id: 456, user_id: 789, message: [{ type: 'text', data: { text } }] });
test('financial commands have exactly one business destination', () => {
  for (const text of ['签到', '/签到', '余额', '/转账 123 1', '@nickname', '/未来业务']) assert.equal(routeEvent(event(text), c), text === '@nickname' ? 'chat' : 'business');
});
test('chat and clear use DSH; foreign identities and groups are dropped', () => {
  for (const text of ['你好', '/聊天 你好', '/清空']) assert.equal(routeEvent(event(text), c), 'chat');
  assert.equal(routeEvent({ ...event('你好'), self_id: 999 }, c), 'drop');
  assert.equal(routeEvent({ ...event('你好'), group_id: 999 }, c), 'drop');
  assert.equal(routeEvent({ ...event('你好'), user_id: 123 }, c), 'drop');
});
test('binding secrets bypass the language model even for the owner', () => {
  for (const text of ['绑定', 'a1b2c3d4', '12345678901234567890_-AB', 'me@example.com']) assert.equal(routeEvent({ ...event(text), message_type: 'private' }, c), 'business');
  assert.equal(routeEvent({ ...event('你好呀'), message_type: 'private' }, c), 'chat');
  assert.equal(routeEvent({ ...event('你好呀'), message_type: 'private', user_id: 999 }, c), 'business');
});

test('malformed gateway messages are rejected without crashing the router', () => {
  for (const value of [null, [], 'hello', { ...event('你好'), message: '你好' }, { ...event('你好'), message: [null] }]) assert.equal(routeEvent(value, c), 'drop');
});
test('complete personal queries reach the site; keyword discussions stay in chat', () => {
  for (const text of ['查下我的余额', '我让你查下我的余额', '帮我查一下余额', '我今天用了多少用量', '我还有多少次抽奖']) assert.equal(routeEvent(event(text), c), 'business');
  for (const text of ['余额是什么', '余额不足该怎么办', '帮我看一下他的余额', '我没让你查下我的余额', '不要查询我的余额', '“查下我的余额”是什么意思', '查下我的余额再转账', '帮我修改余额', '今天的用量怎么统计']) assert.equal(routeEvent(event(text), c), 'chat', text);
  assert.equal(routeEvent({...event('查下我的余额'), message:[{type:'at',data:{qq:'999'}},...event('查下我的余额').message]},c),'drop');
});
test('private users and groups never share a memory scope', () => {
  const a = { bot: '123', sender: '789', chat: '456', message_id: '99', group: true };
  assert.notEqual(scopeKey(a), scopeKey({ ...a, sender: '111' }));
  assert.notEqual(scopeKey(a), scopeKey({ ...a, chat: '777' }));
  assert.notEqual(scopeKey(a), scopeKey({ ...a, group: false }));
  assert.throws(() => scopeKey({ ...a, sender: '../private' }));
  assert.equal(authorized('token', 'token'), true);
  assert.equal(authorized('other', 'token'), false);
});
