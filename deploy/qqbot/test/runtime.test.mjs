import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DeepSeekHarness } from '@deepseek-ai/dsh-sdk-client';

test('the pinned DSH profile makes tool-free model calls and restores session history', { timeout: 90000 }, async () => {
  const dir = mkdtempSync(join(tmpdir(), 'qqbot-runtime-'));
  const requests = [];
  const endpoint = http.createServer(async (req, res) => {
    const chunks = []; for await (const chunk of req) chunks.push(chunk);
    const body = JSON.parse(Buffer.concat(chunks)); requests.push(body);
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    assert.equal(req.url, '/v1/responses');
    const item = { type: 'message', id: 'msg_smoke', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: 'profile-ok', annotations: [] }] };
    const events = [
      { type: 'response.output_item.added', output_index: 0, item: { ...item, status: 'in_progress', content: [] } },
      { type: 'response.content_part.added', output_index: 0, content_index: 0, item_id: item.id, part: { type: 'output_text', text: '', annotations: [] } },
      { type: 'response.output_text.delta', output_index: 0, content_index: 0, item_id: item.id, delta: 'profile-ok' },
      { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response: { id: 'smoke', status: 'completed', output: [item], usage: { input_tokens: 10, output_tokens: 3, total_tokens: 13 } } }
    ];
    for (const value of events) res.write('event: ' + value.type + '\ndata: ' + JSON.stringify(value) + '\n\n');
    res.end();
  });
  endpoint.listen(0, '127.0.0.1'); await once(endpoint, 'listening');
  const options = {
    profile: 'sdk-minimal', patches: [fileURLToPath(new URL('../profile.yml', import.meta.url))],
    dshHome: dir, processCwd: dir, cwd: dir, provider: 'qqbot-site', model: 'qqbot-test', maxTokens: 128,
    initializeTimeoutMs: 30000, requestTimeoutMs: 30000,
    env: { PATH: process.env.PATH, HOME: dir, USERPROFILE: dir, SystemRoot: process.env.SystemRoot,
      DSH_HOME: dir, DSH_TELEMETRY_DISABLED: '1', DSH_SYSTEM_PROMPT: 'Reply briefly.',
      QQBOT_MODEL: 'qqbot-test', QQBOT_MODEL_API_KEY: 'test-only', QQBOT_MODEL_BASE_URL: `http://127.0.0.1:${endpoint.address().port}/v1` }
  };
  let h = new DeepSeekHarness(options);
  try {
    await h.start();
    const session = h.session();
    const first = await session.run('remember qqbot-smoke-marker');
    assert.equal(first.finalResponse, 'profile-ok');
    await h.close(); h = new DeepSeekHarness(options); await h.start();
    const restored = await h.resumeSession(session.id);
    const second = await restored.run('what was the marker');
    assert.equal(second.finalResponse, 'profile-ok');
    assert.equal(requests.length, 2);
    assert.ok(requests.every(r => !r.tools?.length), 'no shell, file, plugin, or agent tools may be exposed');
    assert.ok(JSON.stringify(requests[1].input).includes('qqbot-smoke-marker'));
    assert.ok(requests.every(r => r.max_output_tokens === 128));
  } finally {
    await h.close(); await new Promise(resolve => endpoint.close(resolve)); rmSync(dir, { recursive: true, force: true });
  }
});
