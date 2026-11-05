import { WebSocket, WebSocketServer } from 'ws';
import http from 'node:http';
import { readFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import { authorized, routeEvent } from './policy.mjs';

export function createRouter({ config, onebotToken, astrbotToken, chatUrl = 'ws://astrbot:6199/ws', retryMs = 5000 }) {
  const peers = new Map(), pending = new Map(), timers = new Set();
  let gateway, closing = false;
  const counts = { business: 0, chat: 0, drop: 0, unavailable: 0 };
  const server = http.createServer((req, res) => {
    if (req.url !== '/health') { res.writeHead(404); return res.end(); }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ gateway: gateway?.readyState === WebSocket.OPEN, business: peers.get('business')?.readyState === WebSocket.OPEN, chat: peers.get('chat')?.readyState === WebSocket.OPEN, counts }));
  });
  const listener = new WebSocketServer({ noServer: true, maxPayload: 2 * 1024 * 1024 });
  server.on('upgrade', (req, socket, head) => {
    if (req.url !== '/onebot/ws' || !authorized(req.headers.authorization, 'Bearer ' + onebotToken) || String(req.headers['x-self-id']) !== config.bot_id || req.headers['x-client-role']?.toLowerCase() !== 'universal' || gateway || closing) {
      socket.end('HTTP/1.1 401 Unauthorized\r\n\r\n'); return;
    }
    listener.handleUpgrade(req, socket, head, ws => listener.emit('connection', ws));
  });

  function rejected(peer, echo, message = 'QQ gateway unavailable; operation not replayed') {
    if (peer?.readyState === WebSocket.OPEN) peer.send(JSON.stringify({ status: 'failed', retcode: 1500, data: null, message, echo }));
  }
  function connect(name, url, token, owner) {
    if (closing || gateway !== owner || owner.readyState !== WebSocket.OPEN) return;
    const peer = new WebSocket(url, { headers: { Authorization: 'Bearer ' + token, 'X-Self-ID': config.bot_id, 'X-Client-Role': 'Universal' }, handshakeTimeout: 15000, maxPayload: 2 * 1024 * 1024 });
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
      pending.set(echo, { peer, original: action.echo, expires: Date.now() + 20000 });
      owner.send(JSON.stringify({ ...action, echo }));
    });
    peer.on('error', () => {});
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
        if (item) { pending.delete(String(event.echo)); if (item.peer.readyState === WebSocket.OPEN) item.peer.send(JSON.stringify({ ...event, echo: item.original })); }
        return;
      }
      if (String(event.self_id) !== config.bot_id) return;
      if (event.post_type === 'meta_event') {
        for (const peer of peers.values()) if (peer.readyState === WebSocket.OPEN) peer.send(raw);
        return;
      }
      const route = routeEvent(event, config); counts[route]++;
      const peer = peers.get(route);
      if (peer?.readyState === WebSocket.OPEN) peer.send(raw);
      else if (route !== 'drop') { counts.unavailable++; console.error(JSON.stringify({ event: 'event_not_forwarded', route, message_id: event.message_id })); }
    });
    ws.on('close', () => {
      if (gateway !== ws) return;
      gateway = undefined;
      for (const timer of timers) clearTimeout(timer);
      timers.clear();
      for (const [echo, item] of pending) { pending.delete(echo); rejected(item.peer, item.original); }
      for (const peer of peers.values()) peer.close();
    });
    ws.on('error', () => {});
  });
  const expiry = setInterval(() => {
    for (const [echo, item] of pending) if (item.expires < Date.now()) {
      pending.delete(echo);
      // A sent action with no acknowledgement has unknown outcome: never replay.
      rejected(item.peer, item.original, 'delivery unconfirmed; not replayed');
    }
  }, 5000).unref();
  return {
    server,
    async close() {
      closing = true; clearInterval(expiry);
      for (const timer of timers) clearTimeout(timer);
      for (const peer of peers.values()) peer.terminate();
      gateway?.terminate();
      listener.close();
      await new Promise(resolve => server.close(resolve));
    }
  };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const router = createRouter({
    config: JSON.parse(readFileSync('/run/qqbot/config.json', 'utf8')),
    onebotToken: readFileSync('/run/secrets/onebot-token', 'utf8').trim(),
    astrbotToken: readFileSync('/run/secrets/astrbot-token', 'utf8').trim()
  });
  router.server.listen(6200, '0.0.0.0', () => console.log('QQBot OneBot router listening on 6200'));
  for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, async () => { await router.close(); process.exit(); });
}
