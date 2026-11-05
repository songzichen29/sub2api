export class RuntimeConfig {
  constructor(bootstrap, onebotToken, { fetcher = fetch, stats = () => ({ sessions: 0, messages: 0, summaries: 0 }) } = {}) {
    this.bootstrap = bootstrap; this.token = onebotToken; this.fetcher = fetcher; this.stats = stats;
    this.value = bootstrap.site_config_url ? undefined : bootstrap;
    this.updated = this.value ? Date.now() : 0;
  }
  snapshot() {
    if (!this.value || Date.now() - this.updated > 45000) throw new Error('configuration_not_synchronized');
    return this.value;
  }
  policy() {
    const { model_api_key, persona, ...policy } = this.snapshot();
    return policy;
  }
  async refresh() {
    if (!this.bootstrap.site_config_url) { this.updated = Date.now(); return; }
    const url = new URL(this.bootstrap.site_config_url);
    if (url.protocol !== 'https:' && !['127.0.0.1', 'localhost'].includes(url.hostname)) throw new Error('insecure_configuration_endpoint');
    const headers = { Authorization: 'Bearer ' + this.token, 'X-Self-ID': this.bootstrap.bot_id };
    const response = await this.fetcher(url, { headers, signal: AbortSignal.timeout(8000), redirect: 'error' });
    if (!response.ok) throw new Error('configuration_sync_rejected');
    const config = await response.json();
    if (config.bot_id !== this.bootstrap.bot_id || !Array.isArray(config.groups) || !Array.isArray(config.private_users) || !/^[a-f0-9]{64}$/u.test(config.revision || '')) throw new Error('invalid_configuration');
    this.value = config; this.updated = Date.now();
    const report = await this.fetcher(new URL('status', url), {
      method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' },
      body: JSON.stringify({ revision: config.revision, ...this.stats() }), signal: AbortSignal.timeout(8000), redirect: 'error'
    });
    if (!report.ok) throw new Error('runtime_report_rejected');
  }
  async start() {
    await this.refresh();
    this.timer = setInterval(() => this.refresh().catch(() => console.error(JSON.stringify({ event: 'configuration_sync_failed' }))), 10000).unref();
  }
  close() { clearInterval(this.timer); }
  async tool(body) {
    if (!this.bootstrap.site_config_url) throw new Error('站点工具尚未配置');
    const response=await this.fetcher(new URL('tools',this.bootstrap.site_config_url),{
      method:'POST',headers:{Authorization:'Bearer '+this.token,'X-Self-ID':this.bootstrap.bot_id,'Content-Type':'application/json'},
      body:JSON.stringify(body),signal:AbortSignal.timeout(body.tool==='search_meme'?65000:30000),redirect:'error'
    });
    const value=await response.json();
    if(!response.ok) throw new Error(String(value.error || '站点工具暂不可用').slice(0,300));
    return value;
  }
}

export function bridgeClient(base, token) {
  const call = async (path, body) => {
    const response = await fetch(base + path, { method: body === undefined ? 'GET' : 'POST',
      headers: { 'x-bridge-token': token, 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(8000) });
    if (!response.ok) throw new Error('bridge_request_rejected');
    return response.json();
  };
  return { policy: () => call('/policy'), observe: event => call('/observe', event) };
}
