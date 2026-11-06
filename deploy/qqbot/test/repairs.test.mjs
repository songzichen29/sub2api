import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Store } from '../src/store.mjs';
import { createBridge } from '../src/bridge.mjs';
import { ConversationScheduler } from '../src/scheduler.mjs';
import { executeCapability, validateImages } from '../src/capabilities.mjs';

const png={mimeType:'image/png',data:'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg=='};
const input={bot:'123',chat:'456',sender:'789',group:true,message_id:'1',text:'你好'};
const deferred=()=>{let resolve;const promise=new Promise(r=>{resolve=r;});return {promise,resolve};};
async function fixture(t,run=()=>({finalResponse:'回复'}),options={}) {
  const dir=mkdtempSync(join(tmpdir(),'qq-repairs-')),store=new Store(join(dir,'state.sqlite'),join(dir,'raw'));
  const c={enabled:true,bot_id:'123',groups:['456','457'],private_users:['789'],owners:[],group_admins:[{app_id:'123',group_id:'456',member_id:'789'}],
    model:'test',model_api_key:'test-key',context_window_tokens:16384,max_output_tokens:256,total_daily_chat_limit:1000,daily_chat_limit:1000,cooldown_seconds:0,
    ambient_daily_calls:1000,ambient_daily_replies:1000,ambient_hourly_replies:1000,ambient_media_daily_calls:1000,
    memes_enabled:true,collect_memes:true,image_input_enabled:true,image_generation_enabled:true,web_enabled:true};
  const config={snapshot:()=>c,policy:()=>c},calls=[];let count=0;
  const makeHarness=async options=>{
    const session=id=>({id,run:async blocks=>{const context=JSON.parse(blocks[0].text);calls.push({context,id});return run({context,id,blocks,options,store});}});
    return {start:async()=>{},close:async()=>{},session:()=>session('session-'+ ++count),resumeSession:async id=>session(id)};
  };
  const bridge=createBridge({config,token:'test-token',store,root:dir,makeHarness,...options});
  bridge.server.listen(0,'127.0.0.1');await once(bridge.server,'listening');
  const post=async(path,body,headers={'x-bridge-token':'test-token'})=>{
    const res=await fetch(`http://127.0.0.1:${bridge.server.address().port}${path}`,{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:JSON.stringify(body)});
    return {code:res.status,...await res.json()};
  };
  t.after(async()=>{await bridge.close();store.db.close();rmSync(dir,{recursive:true,force:true});});
  return {store,c,config,calls,bridge,post};
}

test('scheduler preserves per-group order, permits other groups, and cancels queued work',async()=>{
  const s=new ConversationScheduler({concurrency:2,maxPending:1}),a=deferred(),b=deferred(),events=[];
  const first=s.run('a',async()=>{events.push('a1');await a.promise;});
  const second=s.run('b',async()=>{events.push('b1');await b.promise;});
  const cancel=new AbortController();let ran=false;
  const waiting=s.run('a',()=>{ran=true;},cancel.signal);
  const rejected=assert.rejects(waiting,/turn_expired/);
  await assert.rejects(s.run('c',()=>{}),/queue_full/);
  cancel.abort();await rejected;
  const third=s.run('a',()=>events.push('a2'));
  await Promise.resolve();assert.deepEqual(events,['a1','b1']);
  b.resolve();await second;assert.equal(s.waiting,1);
  a.resolve();await Promise.all([first,third]);assert.deepEqual(events,['a1','b1','a2']);assert.equal(ran,false);
  await s.close();
});

test('concurrent group runtimes isolate sessions and tool tokens',async t=>{
  const entered=deferred(),release=deferred(),workers=[];
  const f=await fixture(t,async w=>{workers.push(w);if(workers.length===2)entered.resolve();await release.promise;return {finalResponse:'回复'};});
  const jobs=[f.post('/turn',input),f.post('/turn',{...input,chat:'457',message_id:'2'})];
  await entered.promise;
  assert.equal(f.bridge.stats().active,2);
  assert.notEqual(workers[0].options.dshHome,workers[1].options.dshHome);
  const wrong=await f.post('/tools/execute',{name:'collect_meme',args:{image_index:1,tags:'cat'},session:workers[1].id,call_id:'wrong'},{'x-tool-token':workers[0].options.env.QQBOT_TOOL_TOKEN});
  assert.equal(wrong.code,400);
  release.resolve();assert.ok((await Promise.all(jobs)).every(r=>r.code===200));
});

test('quotes reach the model and partial max-token output stays usable',async t=>{
  const f=await fixture(t,()=>({finalResponse:'这是已有内容',events:[{type:'turn/end',data:{reason:{kind:'max_tokens'}}}]}));
  const quote={message_id:'-77',sender:'888',name:'另一个人',text:'下周一见',available:true};
  const result=await f.post('/turn',{...input,quote});
  assert.equal(result.code,200);assert.match(result.text,/这是已有内容.*\n\n.*输出上限/u);
  assert.deepEqual(f.calls[0].context.quoted_message,quote);
});

test('unsent replies are never reused; only valid receipts promote confirmed memory',async t=>{
  const f=await fixture(t);
  const a=await f.post('/turn',input);
  assert.equal(f.store.conversation('123:group:456'),undefined);
  const b=await f.post('/turn',{...input,message_id:'2'});
  assert.notEqual(a.session,b.session);
  await f.post('/delivery/claim',{id:a.id});
  assert.equal((await f.post('/delivery/finish',{id:a.id,status:'sent'})).code,400);
  assert.equal((await f.post('/delivery/finish',{id:a.id,status:'sent',message_ids:['101']})).code,200);
  assert.equal(f.store.conversation('123:group:456'),undefined,'a late older reply must not replace a newer model session');
  await f.post('/delivery/claim',{id:b.id});
  await f.post('/delivery/receipt',{id:b.id,message_ids:['102']});
  await f.post('/delivery/finish',{id:b.id,status:'sent',message_ids:['102']});
  assert.equal(f.store.conversation('123:group:456').session_id,b.session);
  const c=await f.post('/turn',{...input,message_id:'3'});assert.equal(c.session,b.session);
  f.c.enabled=false;
  assert.equal((await f.post('/delivery/claim',{id:c.id})).claimed,false);
  assert.equal(f.store.conversation('123:group:456'),undefined);
  assert.equal(f.store.db.prepare('SELECT COUNT(*) AS n FROM pending_conversations').get().n,0);
  assert.equal(f.bridge.stats().daily_sent,2);
});

test('a group administrator cannot clear other groups, but can clear their own',async t=>{
  const f=await fixture(t);
  for(const group of ['456','457'])f.store.observe('123:group:'+group,{event_id:'original',sender:'888',text:'保留的记录'});
  const other=await f.post('/turn',{...input,chat:'457',text:'/清空'});
  assert.match(other.text,/请由群管理员/);assert.equal(f.store.history('123:group:457').length,1);
  const own=await f.post('/turn',{...input,text:'/清空'});
  assert.match(own.text,/已重置/);assert.equal(f.store.history('123:group:456').length,0);
});

test('delivery quotas and live group policy reject replies without promoting them',async t=>{
  const f=await fixture(t);f.c.ambient_daily_replies=0;
  const a=await f.post('/turn',{...input,ambient:true});
  assert.equal((await f.post('/delivery/claim',{id:a.id})).error,'rate_limited');
  assert.equal(f.store.conversation('123:group:456'),undefined);
  const b=await f.post('/turn',{...input,message_id:'2'});
  f.c.group_policies=[{group_id:'456',disable_chat:true}];
  assert.equal((await f.post('/delivery/claim',{id:b.id})).claimed,false);
  assert.equal(f.store.diagnostic(b.id).status,'cancelled');
  assert.equal(f.bridge.stats().daily_failed,2);
});

test('runtime restart drops pending sessions and classifies interrupted sends without losing receipts',async t=>{
  const dir=mkdtempSync(join(tmpdir(),'qq-restart-'));
  let store=new Store(join(dir,'state.sqlite'),join(dir,'raw'));
  t.after(()=>{store.db.close();rmSync(dir,{recursive:true,force:true});});
  store.begin('a','123:group:456');store.finish('a','completed','not confirmed');
  store.recordTurn('a','123:group:456','789','1',Date.now()+10000);
  store.stageConversation('a','123:group:456','session',1,20);
  store.claimDelivery('a');store.recordStatus('a','sending');store.recordReceipts('a',['-88']);
  store.db.close();store=new Store(join(dir,'state.sqlite'),join(dir,'raw'));
  assert.equal(store.delivery('a').status,'unconfirmed');
  assert.equal(store.referenceInScope('123:group:456','-88'),true);
  assert.equal(store.conversation('123:group:456'),undefined);
  assert.equal(store.db.prepare('SELECT COUNT(*) AS n FROM pending_conversations').get().n,0);
  assert.equal(store.runtimeStats().unconfirmed,1);
});

test('expired queued and in-flight requests leave no promotable session',async t=>{
  const f=await fixture(t,()=>new Promise(()=>{}),{turnTimeoutMs:100});
  const expired=await f.post('/turn',{...input,deadline_at:Date.now()-1});
  assert.equal(expired.error,'turn_expired');assert.equal(f.calls.length,0);
  const result=await f.post('/turn',{...input,message_id:'2'});
  assert.equal(result.error,'turn_expired');assert.equal(f.store.conversation('123:group:456'),undefined);
  assert.equal(f.bridge.stats().daily_failed,2);
});

test('prepared media can be delivered without text, with permission rechecked and partial receipts retained',async t=>{
  const f=await fixture(t,({store})=>{const turn=store.db.prepare('SELECT id FROM turns ORDER BY rowid DESC LIMIT 1').get();store.addMedia(turn.id,png);return {finalResponse:''};});
  const a=await f.post('/turn',input);assert.equal(a.code,200);assert.equal(a.text,'');assert.equal(a.media.length,1);
  await f.post('/delivery/claim',{id:a.id});await f.post('/delivery/receipt',{id:a.id,message_ids:['-101']});
  await f.post('/delivery/finish',{id:a.id,status:'partial',message_ids:['-101']});
  assert.equal(f.store.referenceInScope('123:group:456','-101'),true);assert.equal(f.store.referenceInScope('123:group:457','-101'),false);
  assert.equal(f.store.conversation('123:group:456'),undefined);
  const b=await f.post('/turn',{...input,message_id:'2'});f.c.memes_enabled=false;
  assert.equal((await f.post('/delivery/claim',{id:b.id})).claimed,false);
  assert.equal(f.bridge.stats().unconfirmed,1);
});

test('image selection never switches members or failed quote, preserves indices and curated tags',async t=>{
  const f=await fixture(t),scope='123:group:456';
  f.store.cacheImages(scope,'old',[png],'888');
  const context={input:{...input,text:'收藏这张图片'},id:'turn',scope,imageCutoff:Date.now()};
  const deps={config:f.config,store:f.store,siteTool:async b=>b};
  const collect=()=>executeCapability({name:'collect_meme',args:{image_index:1,tags:'手工标签'}},context,deps);
  await assert.rejects(collect(),/不会改用其他成员/);
  f.store.cacheImages(scope,'mine',[png],'789');context.imageCutoff=Date.now();
  await collect();
  f.store.collectMeme(scope,png,'自动标签',{automatic:true});assert.equal(f.store.findMeme(scope,'手工标签').description,'手工标签');
  context.input.quote={available:false,message_id:'missing'};await assert.rejects(collect(),/不会改用其他成员/);
  f.store.cacheImages(scope,'1',[{...png,source_index:3}],'789');
  await assert.rejects(collect(),/未读取成功/);
  const edit=await executeCapability({name:'edit_image',args:{image_index:3,prompt:'改成蓝色背景'}},context,deps);
  assert.deepEqual(edit.source_image,png);assert.equal(edit.member_id,'789');assert.equal(edit.message_id,'1');
  assert.equal(validateImages(Array.from({length:4},(_,i)=>({...png,source_index:i+1}))).length,4);
  assert.throws(()=>validateImages([{...png,source_index:2},{...png,source_index:2}]),/invalid_image/);
});

test('vision off still supplies explicit tool images, successful meme searches are cached and cache cleanup preserves collection',async t=>{
  const f=await fixture(t);f.c.image_input_enabled=false;
  await f.post('/turn',{...input,text:'收藏这张图片',image_count:1,image_loaded_count:1,tool_images:[png]});
  assert.equal(f.calls[0].context.attachments.forwarded,0);assert.equal(f.calls[0].context.attachments.loaded,1);
  const scope='123:group:456',context={input:{...input,text:'发开心表情包'},id:'a',scope};let searches=0;
  const deps={config:f.config,store:f.store,siteTool:async(_body,options)=>{searches++;assert.ok(options.timeoutMs<=60000);assert.ok(options.signal);return png;}};
  await executeCapability({name:'send_meme',args:{keyword:'开心'}},context,deps);
  await executeCapability({name:'send_meme',args:{keyword:'开心'}},{...context,id:'b'},deps);
  assert.equal(searches,1);
  f.store.collectMeme(scope,png,'收藏');f.store.pruneMedia(Date.now()+8*86400000);
  assert.equal(f.store.cachedMeme(scope,'开心'),undefined);assert.equal(f.store.cachedImages(scope,'1').length,0);assert.ok(f.store.findMeme(scope,'收藏'));
});
