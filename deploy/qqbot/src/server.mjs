import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { Store } from './store.mjs';
import { RuntimeConfig } from './runtime-config.mjs';
import { migrateMemory } from './memory.mjs';
import { createBridge } from './bridge.mjs';

const root = process.env.QQBOT_DATA || '/data';
const bootstrap = JSON.parse(readFileSync(process.env.QQBOT_CONFIG || '/run/qqbot/config.json', 'utf8'));
const token = readFileSync('/run/secrets/bridge-token', 'utf8').trim();
const onebotToken = readFileSync('/run/secrets/onebot-token', 'utf8').trim();
const store = new Store(join(root, 'state.sqlite'), join(root, 'memory/raw'));
let bridge;
const config = new RuntimeConfig(bootstrap, onebotToken, { stats: () => bridge ? bridge.stats() : {...store.stats(),...store.runtimeStats(),queued:0,active:0} });
await config.start();
migrateMemory(store, config.snapshot());
store.seedMeters(bootstrap.initial_usage || []);
bridge = createBridge({ config, token, store, root, makeHarness: async options => {
  const { DeepSeekHarness } = await import('@deepseek-ai/dsh-sdk-client');
  return new DeepSeekHarness(options);
} });
bridge.server.listen(8787, '0.0.0.0', () => console.log('QQBot DSH bridge listening on 8787'));
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, async () => {
  config.close(); await bridge.close(); store.db.close(); process.exit();
});
