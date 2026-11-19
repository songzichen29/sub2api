import test from 'node:test';
import assert from 'node:assert/strict';
import { RuntimeConfig } from '../src/runtime-config.mjs';
import { errorFields } from '../src/diagnostics.mjs';

test('runtime errors retain the actual failing stage without logging credentials or response bodies',async()=>{
  const bootstrap={bot_id:'123',site_config_url:'https://example.invalid/runtime/config'};
  const valid={bot_id:'123',groups:[],private_users:[],revision:'a'.repeat(64)};
  const network=new RuntimeConfig(bootstrap,'secret',{fetcher:async()=>{throw new TypeError('fetch failed at https://private/?key=secret',{cause:{code:'ECONNRESET'}});}});
  await assert.rejects(network.refresh(),error=>{
    const fields=errorFields(error);
    assert.deepEqual(fields,{code:'operation_failed',stage:'config',type:'TypeError',status:undefined,cause:'ECONNRESET'});
    assert.ok(!JSON.stringify(fields).includes('secret'));
    return true;
  });
  const status=new RuntimeConfig(bootstrap,'secret',{fetcher:async(url,options)=>options.method==='POST'?{ok:false,status:503}:{ok:true,json:async()=>valid}});
  await assert.rejects(status.refresh(),error=>error.stage==='status' && error.status===503);
  const ingress=new RuntimeConfig(bootstrap,'secret',{fetcher:async(url,options)=>{
    assert.equal(url.pathname,'/runtime/ingress');
    assert.equal(options.headers['X-Self-ID'],'123');
    assert.equal(JSON.parse(options.body).phase,'route');
    return {ok:false,status:503,json:async()=>({error:'binding_state_unavailable'})};
  }});
  await assert.rejects(ingress.checkEvent({phase:'route',event:{}}),error=>errorFields(error).code==='binding_state_unavailable' && error.stage==='ingress');
});
