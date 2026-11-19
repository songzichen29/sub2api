import { errorFields } from './diagnostics.mjs';

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
  async request(stage, url, options) {
    try {
      return await this.fetcher(url, options);
    } catch (error) {
      throw Object.assign(error, { stage });
    }
  }
  async refresh() {
    if (!this.bootstrap.site_config_url) { this.updated = Date.now(); return; }
    const url = new URL(this.bootstrap.site_config_url);
    if (url.protocol !== 'https:' && !['127.0.0.1', 'localhost'].includes(url.hostname)) throw new Error('insecure_configuration_endpoint');
    const headers = { Authorization: 'Bearer ' + this.token, 'X-Self-ID': this.bootstrap.bot_id };
    const response = await this.request('config', url, { headers, signal: AbortSignal.timeout(8000), redirect: 'error' });
    if (!response.ok) throw Object.assign(new Error('configuration_sync_rejected'), { stage: 'config', status: response.status });
    const config = await response.json().catch(error=>{throw Object.assign(error,{stage:'config',status:response.status});});
    if (config.bot_id !== this.bootstrap.bot_id || !Array.isArray(config.groups) || !Array.isArray(config.private_users) || !/^[a-f0-9]{64}$/u.test(config.revision || '')) throw Object.assign(new Error('invalid_configuration'),{stage:'config'});
    this.value = config; this.updated = Date.now();
    const report = await this.request('status', new URL('status', url), {
      method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' },
      body: JSON.stringify({ revision: config.revision, ...this.stats() }), signal: AbortSignal.timeout(8000), redirect: 'error'
    });
    if (!report.ok) throw Object.assign(new Error('runtime_report_rejected'), { stage: 'status', status: report.status });
  }
  async start() {
    await this.refresh();
    this.timer = setInterval(() => this.refresh().catch(error => console.error(JSON.stringify({ event: 'configuration_sync_failed', ...errorFields(error) }))), 10000).unref();
  }
  close() { clearInterval(this.timer); }
  async checkEvent(body) {
    const response = await this.request('ingress', new URL('ingress', this.bootstrap.site_config_url), {
      method: 'POST', headers: { Authorization: 'Bearer ' + this.token, 'X-Self-ID': this.bootstrap.bot_id, 'Content-Type': 'application/json' },
      body: JSON.stringify(body), signal: AbortSignal.timeout(8000), redirect: 'error'
    });
    const result = await response.json().catch(error=>{throw Object.assign(error,{stage:'ingress',status:response.status});});
    if (!response.ok) throw Object.assign(new Error('event_check_failed'), { code: result.error, stage: 'ingress', status: response.status });
    return result;
  }
  async tool(body,{signal,timeoutMs}={}) {
    if (!this.bootstrap.site_config_url) throw new Error('站点工具尚未配置');
    const response=await this.request('tools',new URL('tools',this.bootstrap.site_config_url),{
      method:'POST',headers:{Authorization:'Bearer '+this.token,'X-Self-ID':this.bootstrap.bot_id,'Content-Type':'application/json'},
      body:JSON.stringify(body),signal:AbortSignal.any([AbortSignal.timeout(timeoutMs || (body.tool==='search_meme'?65000:30000)),...(signal?[signal]:[])]),redirect:'error'
    });
    const value=await response.json().catch(error=>{throw Object.assign(error,{stage:'tools',status:response.status});});
    if(!response.ok) throw Object.assign(new Error(String(value.error || '站点工具暂不可用').slice(0,300)), { code: value.code || 'site_tool_rejected', stage: 'tools', status: response.status });
    return value;
  }
}

export function bridgeClient(base, token) {
  const call = async (path, body) => {
    const response = await fetch(base + path, { method: body === undefined ? 'GET' : 'POST',
      headers: { 'x-bridge-token': token, 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(8000) });
    if (!response.ok) throw Object.assign(new Error('bridge_request_rejected'), { stage: path, status: response.status });
    return response.json();
  };
  return { policy: () => call('/policy'), observe: event => call('/observe', event), checkEvent: body => call('/ingress', body) };
}
