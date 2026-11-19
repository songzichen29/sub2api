import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { once } from 'node:events';
import { parseVoiceCommand, parseVoiceRequest, synthesizeVoice } from '../src/voice.mjs';
import { routeEvent } from '../src/policy.mjs';

// 10ms 单声道 PCM WAV，测试传输真实音频字节而非供应商服务。
const wav = Buffer.alloc(204);
wav.write('RIFF'); wav.writeUInt32LE(196, 4); wav.write('WAVEfmt ', 8);
wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20); wav.writeUInt16LE(1, 22);
wav.writeUInt32LE(8000, 24); wav.writeUInt32LE(16000, 28); wav.writeUInt16LE(2, 32);
wav.writeUInt16LE(16, 34); wav.write('data', 36); wav.writeUInt32LE(160, 40);

async function endpoint(t, handler) {
  const server = http.createServer(handler);
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  t.after(() => new Promise(resolve => server.close(resolve)));
  return (url, options) => {
    assert.equal(url, 'https://video.chrelyonly.cn/video-api/getVideo');
    return fetch(`http://127.0.0.1:${server.address().port}/video-api/getVideo`, options);
  };
}

test('documented voice commands select fixed model IDs and leave ordinary conversation alone', () => {
  for (const [command, voice, referenceId] of [
    ['御姐语音合成 你好', '御姐', '8a88ef7d77d748e59879d8994e216e01'],
    ['/曼波语音合成：你好', '曼波', '561fcedfdf0e4e1399d1bc4930d50c0e'],
    ['Nico语音合成 你好', 'Nico', '1aa39c8196a349cfb459ef6e0eb2c7bb'],
    ['可莉语音合成 你好', '可莉', 'a54b3b693c6a4218bcd758a5b8b1fceb'],
    ['语音合成 你好', '默认', 'fbe02f8306fc4d3d915e9871722a39d5'],
    ['/语音合成：你好', '默认', 'fbe02f8306fc4d3d915e9871722a39d5']
  ]) assert.deepEqual(parseVoiceCommand(command), { voice, referenceId, text: '你好' });
  assert.equal(parseVoiceCommand('/语音合成').text, '');
  for (const text of ['什么是语音合成', '你能语音合成吗', '不要御姐语音合成 你好', '未知语音合成 你好', '你好']) {
    assert.equal(parseVoiceCommand(text), undefined, text);
  }
});

test('voice commands route to chat while binding, account queries and group boundaries remain intact', () => {
  const config = { bot_id: '123', groups: ['456'], private_users: ['789'], business_commands: ['余额', '绑定'] };
  const event = text => ({ self_id: 123, user_id: 789, post_type: 'message', message_type: 'group', group_id: 456, message: [{ type: 'text', data: { text } }] });
  for (const text of ['/御姐语音合成 你好', '语音合成 你好', '/语音合成']) {
    assert.equal(routeEvent(event(text), config), 'chat');
    assert.equal(routeEvent(event(text), { ...config, enabled: false }), 'drop');
    assert.equal(routeEvent(event(text), { ...config, chat_disabled: true }), 'drop');
    assert.equal(routeEvent({ ...event(text), group_id: 999 }, config), 'drop');
    assert.equal(routeEvent({ ...event(text), message: [{ type: 'at', data: { qq: '999' } }, ...event(text).message] }, config), 'drop');
  }
  assert.equal(routeEvent(event('查下我的余额'), config), 'business');
  assert.equal(routeEvent({ ...event('/语音合成 你好'), message_type: 'private', user_id: 999 }, config), 'business');
});

test('current natural requests distinguish spoken answers, literal reading and explicit text overrides', () => {
  for (const text of ['用语音回复我，为什么天空是蓝色的？', '今天怎么样？请用语音回答我', '你能不能用语音给我讲个故事', '给我发条语音吧', '语音说一下你的看法', '用语音回复，别用文字', '你用语音回我', '我想听你用语音讲个故事']) {
    assert.equal(parseVoiceRequest(text)?.mode, 'reply', text);
    assert.equal(parseVoiceRequest(text).referenceId, 'fbe02f8306fc4d3d915e9871722a39d5');
  }
  assert.equal(parseVoiceRequest('麻烦你用御姐的声音回答我，讲个笑话').voice, '御姐');
  for (const text of ['这段话念给我听', '把引用的内容读给我听', '请朗读：你好', '把“你好，世界。”念给我听', '语音合成 你好']) {
    assert.equal(parseVoiceRequest(text)?.mode, 'read', text);
  }
  assert.equal(parseVoiceRequest('把“你好，世界。”念给我听').text, '你好，世界。');
  assert.equal(parseVoiceRequest('用御姐的声音把这句话念给我听：你好').text, '你好');
  for (const text of ['你好', '不要用语音回复我', '请不要用语音回答', '什么是语音回复？', '你支持语音回复吗', '他想让你用语音回复他', '“用语音回复我”是什么意思', '用语音回复我，还是用文字吧', '请用语音回答，算了，不需要语音了']) {
    assert.equal(parseVoiceRequest(text), undefined, text);
  }
});

test('the TTS client posts the documented JSON and splits every character without breaking emoji', async t => {
  const requests = [];
  const fetcher = await endpoint(t, async (req, res) => {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    requests.push(JSON.parse(Buffer.concat(chunks)));
    assert.equal(req.method, 'POST');
    assert.equal(req.url, '/video-api/getVideo');
    assert.equal(req.headers['content-type'], 'application/json');
    assert.equal(req.headers.authorization, undefined);
    assert.equal(req.headers['x-bridge-token'], undefined);
    res.writeHead(200, { 'Content-Type': 'application/octet-stream' }); res.end(wav);
  });
  const text = '字'.repeat(499) + '🙂' + '\n你好'.repeat(350);
  const command = parseVoiceCommand('御姐语音合成 ' + text);
  const media = await synthesizeVoice(command, { fetcher, signal: AbortSignal.timeout(5000) });
  assert.equal(requests.length, 4);
  assert.equal(requests.map(body => body.text).join(''), text);
  assert.ok(requests.every(body => body.text.length <= 500 && body.text.isWellFormed()));
  assert.ok(requests.every(body => body.referenceId === '8a88ef7d77d748e59879d8994e216e01'));
  assert.equal(media.length, requests.length);
  assert.ok(media.every(item => item.mimeType === 'audio/wav' && Buffer.from(item.data, 'base64').equals(wav)));
});

test('failed, empty, JSON, HTML, redirect and oversized responses fail explicitly without retry', async t => {
  for (const [name, status, type, body, expected] of [
    ['HTTP error', 503, 'application/json', '{"error":"unavailable"}', /voice_request_failed/u],
    ['JSON error', 200, 'application/json', '{"error":"invalid reference"}', /voice_response_invalid/u],
    ['undocumented URL response', 200, 'application/json', '{"url":"http://127.0.0.1/private"}', /voice_response_invalid/u],
    ['HTML error', 200, 'text/html', '<html>error</html>', /voice_response_invalid/u],
    ['empty audio', 200, 'audio/mpeg', '', /voice_response_invalid/u],
    ['redirect', 302, 'text/plain', '', /fetch failed/u],
    ['oversized', 200, 'audio/wav', Buffer.alloc(6 * 1024 * 1024 + 1), /voice_response_too_large/u]
  ]) await t.test(name, async t => {
    let calls = 0;
    const fetcher = await endpoint(t, (req, res) => {
      calls++;
      res.writeHead(status, { 'Content-Type': type, Location: '/redirected' }); res.end(body);
    });
    await assert.rejects(synthesizeVoice(parseVoiceCommand('语音合成 你好'), { fetcher }), expected);
    assert.equal(calls, 1);
  });
});

test('an empty command never calls TTS and cancellation aborts the active HTTP request', async t => {
  await assert.rejects(synthesizeVoice(parseVoiceCommand('/语音合成')), /voice_text_missing/u);
  let received;
  const started = new Promise(resolve => { received = resolve; });
  const fetcher = await endpoint(t, (req, res) => { received(); res.writeHead(200, { 'Content-Type': 'audio/wav' }); res.write(wav); });
  const controller = new AbortController();
  const audio = synthesizeVoice(parseVoiceCommand('语音合成 你好'), { fetcher, signal: controller.signal });
  await started;
  controller.abort();
  await assert.rejects(audio, { name: 'AbortError' });
});
