import { WebSocket, WebSocketServer } from 'ws';
import http from 'node:http';
import { readFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import { authorized, routeEvent, shouldObserveGroup } from './policy.mjs';
import { bridgeClient } from './runtime-config.mjs';
import { errorFields } from './diagnostics.mjs';
import { chatAllowed } from './chat-policy.mjs';

export function createRouter({ config, onebotToken, astrbotToken, bridgeToken, policy = async () => config, chatUrl = 'ws://astrbot:6199/ws', retryMs = 5000, observe, checkEvent, actionTimeoutMs = 20000 }) {
  // A 6 MiB image becomes roughly 8 MiB when OneBot carries it as base64 JSON.
  // Keep the limit bounded while allowing the configured image input size through.
  const onebotMaxPayload = 12 * 1024 * 1024;
  const peers = new Map(), pending = new Map(), timers = new Set();
  let gateway, closing = false;
  const observations = new Map();
  const counts = { business: 0, chat: 0, drop: 0, observed: 0, unavailable: 0 };
  const server = http.createServer(async (req, res) => {
    if (req.method === 'POST' && req.url === '/send-group' && bridgeToken) {
      const reply = (status, value) => {
        if (res.destroyed || res.writableEnded) return;
        res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
        res.end(JSON.stringify(value));
      };
      if (!authorized(req.headers['x-bridge-token'], bridgeToken)) return reply(401, { error: 'unauthorized' });
      const request = new AbortController();
      req.once('aborted', () => request.abort());
      res.once('close', () => request.abort());
      try {
        const chunks = [];
        let size = 0;
        for await (const part of req) {
          size += part.length;
          if (size > onebotMaxPayload) throw new Error('body_too_large');
          chunks.push(part);
        }
        const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
        const current = await policy();
        // 权限同步期间调用方可能已经取消回合，此时绝不能再执行 QQ 发送。
        if (request.signal.aborted || res.destroyed) return;
        const source = { bot: body.bot, sender: body.sender, chat: body.sender, group: false };
        if (!chatAllowed(current, source) || !chatAllowed(current, { ...source, group: true, chat: body.group_id })) return reply(403, { error: '来源私聊或目标群权限已关闭' });
        const group = Number(body.group_id), segment = body.segment;
        if (!Number.isSafeInteger(group) || group <= 0) throw new Error('invalid_group_id');
        if (!segment || !['text', 'record'].includes(segment.type)) throw new Error('invalid_segment');
        const text = segment.data?.text, file = segment.data?.file;
        if (segment.type === 'text' && (typeof text !== 'string' || !text.length || Array.from(text).length > 1200)) throw new Error('invalid_text');
        if (segment.type === 'record') {
          if (typeof file !== 'string' || !/^base64:\/\/[A-Za-z0-9+/]+={0,2}$/u.test(file)) throw new Error('invalid_audio');
          const encoded = file.slice(9), audio = Buffer.from(encoded, 'base64');
          const wav = audio.subarray(0, 4).toString('ascii') === 'RIFF' && audio.subarray(8, 12).toString('ascii') === 'WAVE';
          const mp3 = audio.subarray(0, 3).toString('ascii') === 'ID3' || (audio[0] === 0xff && (audio[1] & 0xe0) === 0xe0);
          if (audio.length > 6 * 1024 * 1024 || audio.toString('base64') !== encoded || (!wav && !mp3)) throw new Error('invalid_audio');
        }
        if (closing || gateway?.readyState !== WebSocket.OPEN || pending.size >= 300) return reply(503, { error: 'QQ gateway unavailable; operation not replayed' });
        const echo = randomUUID();
        pending.set(echo, { expires: Date.now() + actionTimeoutMs, reply: event => {
          const message = event.data?.message_id;
          if (event.status !== 'ok' || event.retcode !== 0 || !/^-?\d{1,24}$/u.test(String(message)) || String(message) === '0' || (typeof message === 'number' && !Number.isSafeInteger(message))) return reply(502, { error: 'QQ 发送回执未确认，未自动重发' });
          reply(200, { message_id: String(message) });
        } });
        res.on('close', () => pending.delete(echo));
        gateway.send(JSON.stringify({ action: 'send_group_msg', params: { group_id: group, message: [{ type: segment.type, data: segment.type === 'text' ? { text } : { file } }] }, echo }));
      } catch (error) { reply(400, { error: error.message }); }
      return;
    }
    if (req.url !== '/health') { res.writeHead(404); return res.end(); }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ gateway: gateway?.readyState === WebSocket.OPEN, business: peers.get('business')?.readyState === WebSocket.OPEN, chat: peers.get('chat')?.readyState === WebSocket.OPEN, pending_scopes: observations.size, counts }));
  });
  const listener = new WebSocketServer({ noServer: true, maxPayload: onebotMaxPayload });
  server.on('upgrade', (req, socket, head) => {
    if (req.url !== '/onebot/ws' || !authorized(req.headers.authorization, 'Bearer ' + onebotToken) || String(req.headers['x-self-id']) !== config.bot_id || req.headers['x-client-role']?.toLowerCase() !== 'universal' || gateway || closing) {
      socket.end('HTTP/1.1 401 Unauthorized\r\n\r\n'); return;
    }
    listener.handleUpgrade(req, socket, head, ws => listener.emit('connection', ws));
  });

  function rejected(peer, echo, message = 'QQ gateway unavailable; operation not replayed') {
    if (peer?.readyState === WebSocket.OPEN) peer.send(JSON.stringify({ status: 'failed', retcode: 1500, data: null, message, echo }));
  }
  function rejectPending(item, message) {
    if (item.reply) return item.reply({ status: 'failed', retcode: 1500 });
    rejected(item.peer, item.original, message);
  }
  function connect(name, url, token, owner) {
    if (closing || gateway !== owner || owner.readyState !== WebSocket.OPEN) return;
    const peer = new WebSocket(url, { headers: { Authorization: 'Bearer ' + token, 'X-Self-ID': config.bot_id, 'X-Client-Role': 'Universal' }, handshakeTimeout: 15000, maxPayload: 12 * 1024 * 1024 });
    peers.set(name, peer);
    peer.on('open', () => {
      if (gateway !== owner || closing) return peer.close();
      console.log(JSON.stringify({ event: 'upstream_connected', peer: name }));
      peer.send(JSON.stringify({ time: Math.floor(Date.now() / 1000), self_id: Number(config.bot_id), post_type: 'meta_event', meta_event_type: 'lifecycle', sub_type: 'connect' }));
    });
    peer.on('message', raw => {
      let action; try { action = JSON.parse(raw.toString()); } catch { return; }
      if (!action || typeof action.action !== 'string') return;
      if (gateway !== owner || owner.readyState !== WebSocket.OPEN || pending.size >= 300) return rejected(peer, action.echo);
      const echo = randomUUID();
      pending.set(echo, { peer, original: action.echo, expires: Date.now() + actionTimeoutMs });
      owner.send(JSON.stringify({ ...action, echo }));
    });
    peer.on('error', error => console.error(JSON.stringify({ event: 'upstream_connection_failed', peer: name, ...errorFields(error) })));
    peer.on('close', () => {
      if (peers.get(name) === peer) peers.delete(name);
      for (const [echo, item] of pending) if (item.peer === peer) pending.delete(echo);
      if (!closing && gateway === owner && owner.readyState === WebSocket.OPEN) {
        const timer = setTimeout(() => { timers.delete(timer); connect(name, url, token, owner); }, retryMs);
        timers.add(timer);
      }
    });
  }
  listener.on('connection', ws => {
    gateway = ws;
    // Do not occupy the production business socket before NapCat switches here.
    connect('business', config.business_ws_url, onebotToken, ws);
    connect('chat', chatUrl, astrbotToken, ws);
    ws.on('message', raw => {
      let event; try { event = JSON.parse(raw.toString()); } catch { return; }
      if (!event || typeof event !== 'object' || Array.isArray(event)) return;
      if (Object.hasOwn(event, 'echo')) {
        const item = pending.get(String(event.echo));
        if (item) {
          pending.delete(String(event.echo));
          if (item.reply) return item.reply(event);
          if (item.peer.readyState === WebSocket.OPEN) item.peer.send(JSON.stringify({ ...event, echo: item.original }));
        }
        return;
      }
      if (String(event.self_id) !== config.bot_id) return;
      // Only this router decides whether an event may execute business logic.
      delete event.sub2api_observe_only;
      delete event.sub2api_received_at;
      if (event.post_type === 'message') event.sub2api_received_at = Date.now();
      const clean = JSON.stringify(event);
      if (event.post_type === 'meta_event') {
        for (const peer of peers.values()) if (peer.readyState === WebSocket.OPEN) peer.send(clean);
        return;
      }
      const dispatch = async () => {
        const initialRoute = routeEvent(event, config);
        const gate = checkEvent && event.post_type === 'message' && (event.message_type === 'private' || shouldObserveGroup(event, config))
          ? await checkEvent({ phase: 'route', event }) : { allowed: true };
        const route = !gate.allowed ? 'drop' : gate.business === true ? 'business' : initialRoute;
        counts[route]++;
        if (!gate.allowed) {
          console.info(JSON.stringify({event:'event_blocked',message_id:event.message_id,reason:gate.reason || 'scope_denied'}));
          return;
        }
        if (route !== 'business' && shouldObserveGroup(event, config)) {
          const business = peers.get('business');
          if (business?.readyState === WebSocket.OPEN) {
            business.send(JSON.stringify({ ...event, sub2api_observe_only: true })); counts.observed++;
          } else { counts.unavailable++; console.error(JSON.stringify({ event: 'observation_not_forwarded', message_id: event.message_id })); }
        }
        const forward = () => {
          if (gateway !== ws) return;
          const peer = peers.get(route);
          if (peer?.readyState === WebSocket.OPEN) peer.send(clean);
          else if (route !== 'drop') { counts.unavailable++; console.error(JSON.stringify({ event: 'event_not_forwarded', route, message_id: event.message_id })); }
        };
        if (observe && shouldObserveGroup(event, config)) {
          await observe(event);
        }
        forward();
      };
      const scope = event.message_type === 'private' ? `private:${event.user_id}` : `group:${event.group_id}`;
      const previous = observations.get(scope) || Promise.resolve();
      const work = previous.then(dispatch).catch(error => {
        counts.unavailable++;
        console.error(JSON.stringify({ event: 'event_dispatch_failed', message_id: event.message_id, ...errorFields(error) }));
      }).finally(() => { if (observations.get(scope) === work) observations.delete(scope); });
      observations.set(scope, work);
    });
    ws.on('close', () => {
      if (gateway !== ws) return;
      gateway = undefined;
      for (const timer of timers) clearTimeout(timer);
      timers.clear();
      for (const [echo, item] of pending) { pending.delete(echo); rejectPending(item); }
      for (const peer of peers.values()) peer.close();
    });
    ws.on('error', error => console.error(JSON.stringify({ event: 'gateway_connection_failed', ...errorFields(error) })));
  });
  const expiry = setInterval(() => {
    for (const [echo, item] of pending) if (item.expires < Date.now()) {
      pending.delete(echo);
      // A sent action with no acknowledgement has unknown outcome: never replay.
      rejectPending(item, 'delivery unconfirmed; not replayed');
    }
  }, Math.min(5000, actionTimeoutMs)).unref();
  return {
    server,
    async close() {
      closing = true; clearInterval(expiry);
      for (const [echo, item] of pending) { pending.delete(echo); rejectPending(item); }
      for (const timer of timers) clearTimeout(timer);
      for (const peer of peers.values()) peer.terminate();
      gateway?.terminate();
      listener.close();
      await new Promise(resolve => server.close(resolve));
    }
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const config = JSON.parse(readFileSync('/run/qqbot/config.json', 'utf8'));
  const bridgeToken = readFileSync('/run/secrets/bridge-token', 'utf8').trim();
  const client = bridgeClient('http://bridge:8787', bridgeToken);
  Object.assign(config, await client.policy());
  const policyTimer = setInterval(() => client.policy().then(policy => Object.assign(config, policy)).catch(error => console.error(JSON.stringify({ event: 'policy_sync_failed', ...errorFields(error) }))), 5000).unref();
  const router = createRouter({
    config, bridgeToken, policy: () => client.policy(),
    onebotToken: readFileSync('/run/secrets/onebot-token', 'utf8').trim(),
    astrbotToken: readFileSync('/run/secrets/astrbot-token', 'utf8').trim(), observe: event => client.observe(event), checkEvent: body => client.checkEvent(body)
  });
  router.server.listen(6200, '0.0.0.0', () => console.log('QQBot OneBot router listening on 6200'));
  for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, async () => { clearInterval(policyTimer); await router.close(); process.exit(); });
}
