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

test('poke interactions invoke the conversation model with the current group history',async t=>{
  const f=await fixture(t,()=>({finalResponse:'怎么啦，接着聊刚才的开黑？'}));
  f.store.observe('123:group:456',{event_id:'msg:prior',sender:'789',text:'刚才在聊开黑'});
  const result=await f.post('/turn',{...input,message_id:'poke-event',text:'[戳一戳事件] 当前发送者刚刚戳了你一下，请结合当前会话自然回应这次互动。',ambient:false});
  assert.equal(result.code,200);
  assert.equal(result.text,'怎么啦，接着聊刚才的开黑？');
  assert.equal(f.calls.length,1);
  assert.equal(f.calls[0].context.verified_sender,'789');
  assert.ok(f.calls[0].context.conversation_memory.messages.some(m=>m.text==='刚才在聊开黑'));
});

test('the model can select voice for semantic requests within the same turn, never historical preferences',async t=>{
  const spoken=[];
  const f=await fixture(t,async ({context,id,options})=>{
    if (!context.text.includes('普通')) {
      const response=await fetch(options.env.QQBOT_TOOL_URL,{method:'POST',headers:{'x-tool-token':options.env.QQBOT_TOOL_TOKEN,'Content-Type':'application/json'},
        body:JSON.stringify({name:'set_reply_mode',args:{mode:'voice'},session:id,call_id:'mode'})});
      const result=await response.json();
      assert.equal(response.status,context.text.includes('不要语音')?400:200);
      if (response.ok) assert.equal(result.status,'selected');
    }
    return {finalResponse:'这是模型实际回答。'};
  },{speech:async command=>{spoken.push(command);return [{mimeType:'audio/mpeg',data:'SUQzBAAAAAAAAA=='}];}});
  for (const [i,text] of ['今天的天气用语音回答我','我不方便看屏幕，能说给我听吗','今天星期几  用语音回答我','语音','不要语音，讲个故事','普通聊天'].entries()) {
    const result=await f.post('/turn',{...input,message_id:String(100+i),text,quote:{available:true,text:'用语音回复我',message_id:'-70'}});
    assert.equal(result.code,200);
    assert.equal(result.media.length,i<4?1:0);
    assert.equal(result.text,i<4?'':'这是模型实际回答。');
    await f.post('/delivery/claim',{id:result.id});
    await f.post('/delivery/finish',{id:result.id,status:'sent',message_ids:[String(-100-i)]});
  }
  assert.equal(f.calls.length,6,'mode selection must not add a classifier model request');
  assert.equal(spoken.length,4);
  assert.ok(spoken.every(command=>command.text==='这是模型实际回答。' && command.referenceId==='fbe02f8306fc4d3d915e9871722a39d5'));
  assert.ok(f.calls.every(call=>call.context.quoted_message.text==='用语音回复我'));
  assert.match(f.calls[3].context.response_mode,/引用后仅说“语音”/u);
});

test('moderation blocks model execution and cancels delivery if recalled while generating',async t=>{
  let allowed=false;
  const checks=[];
  const f=await fixture(t,()=>({finalResponse:'尚未送达'}),{checkEvent:async body=>{checks.push(body);return {allowed};}});
  f.c.moderation_enabled=true;f.c.moderation_groups=['456'];
  const rejected=await f.post('/turn',input);
  assert.equal(rejected.code,400);assert.equal(f.calls.length,0);
  allowed=true;
  const result=await f.post('/turn',{...input,message_id:'2'});
  assert.equal(result.code,200);
  allowed=false;
  assert.equal((await f.post('/delivery/claim',{id:result.id})).claimed,false);
  assert.equal(f.store.history('123:group:456').filter(m=>m.role==='assistant').length,0);
  assert.deepEqual(checks.at(-1).event,{post_type:'message',message_type:'group',self_id:'123',group_id:'456',user_id:'789',message_id:'2'});
});

test('a successful reply-mode tool call cannot make its internal instruction the spoken answer',async t=>{
  const spoken=[];
  const f=await fixture(t,async ({id,options})=>{
    const response=await fetch(options.env.QQBOT_TOOL_URL,{method:'POST',headers:{'x-tool-token':options.env.QQBOT_TOOL_TOKEN,'Content-Type':'application/json'},
      body:JSON.stringify({name:'set_reply_mode',args:{mode:'voice'},session:id,call_id:'mode'})});
    assert.equal((await response.json()).status,'selected');
    return {finalResponse:'调用set_reply_mode选择voice。'};
  },{speech:async command=>{spoken.push(command);return [{mimeType:'audio/mpeg',data:'SUQzBAAAAAAAAA=='}];}});
  const failed=await f.post('/turn',{...input,text:'我不方便看屏幕，能说给我听吗'});
  assert.equal(failed.code,400);
  assert.equal(failed.error,'voice_answer_missing');
  assert.match(failed.message,/没有生成实际回答正文/u);
  assert.equal(spoken.length,0);
  assert.equal(f.store.db.prepare('SELECT COUNT(*) AS n FROM outgoing_media').get().n,0);
  const correction=await f.post('/turn',{...input,message_id:'correction',text:'不要再说 set_reply_mode，用语音回答我今天星期几'});
  assert.equal(correction.error,'voice_answer_missing');
  assert.equal(spoken.length,0);
  const technical=await f.post('/turn',{...input,message_id:'2',text:'用语音解释 set_reply_mode 怎么调用'});
  assert.equal(technical.code,200);
  assert.equal(spoken[0].text,'调用set_reply_mode选择voice。');
  const quoted=await f.post('/turn',{...input,message_id:'3',text:'语音',quote:{available:true,text:'调用set_reply_mode选择voice。',message_id:'old'}});
  assert.equal(quoted.code,200);
  assert.equal(spoken[1].text,'调用set_reply_mode选择voice。');
});

test('spoken answers synthesize model output, preserve receipt-confirmed memory and do not persist into later turns',async t=>{
  const spoken=[];
  const f=await fixture(t,({context})=>{
    if (context.text.includes('用语音')) {
      assert.equal(context.effective_reply.mode,'voice');
      assert.equal(context.quoted_message.text,'一起去爬山吧');
      return {finalResponse:'好呀，周末一起去爬山。'};
    }
    assert.equal(context.effective_reply.mode,'text');
    return {finalResponse:'那就周六出发。'};
  },{speech:async command=>{spoken.push(command);return [{mimeType:'audio/mpeg',data:'SUQzBAAAAAAAAA=='}];}});
  const request={...input,text:'用语音回复我，你觉得这个提议怎么样？',quote:{available:true,text:'一起去爬山吧',message_id:'previous'}};
  const result=await f.post('/turn',request);
  assert.equal(result.code,200);
  assert.equal(result.text,'');
  assert.equal(result.media.length,1);
  assert.equal(spoken[0].text,'好呀，周末一起去爬山。');
  assert.equal(spoken[0].referenceId,'fbe02f8306fc4d3d915e9871722a39d5');
  assert.equal(f.store.history('123:group:456').filter(m=>m.role==='assistant').length,0);
  const duplicate=await f.post('/turn',request);
  assert.equal(duplicate.duplicate,true);
  assert.equal(duplicate.text,'');
  assert.deepEqual(duplicate.media,result.media);
  assert.equal(f.calls.length,1);
  assert.equal(spoken.length,1);
  await f.post('/delivery/claim',{id:result.id});
  await f.post('/delivery/finish',{id:result.id,status:'sent',message_ids:['-100']});
  assert.match(f.store.history('123:group:456').find(m=>m.role==='assistant').text,/好呀，周末一起去爬山。.*\n.*已发送语音/u);
  const next=await f.post('/turn',{...input,message_id:'next',text:'那就定个时间'});
  assert.equal(next.text,'那就周六出发。');
  assert.equal(next.media.length,0);
  assert.equal(spoken.length,1);
  assert.equal(next.session,result.session);
});

test('voice synthesis failures never become text successes and model failures never call TTS',async t=>{
  for (const [name,run,speech,expected] of [
    ['tts',()=>({finalResponse:'模型回答'}),async()=>{throw new Error('fetch failed');},'voice_request_failed'],
    ['model',()=>{throw new Error('upstream unavailable');},async()=>assert.fail('模型失败不合成语音'),'model_unavailable'],
    ['sensitive',()=>({finalResponse:'API Key: sk-1234567890abcdefghijklmnop'}),async()=>assert.fail('敏感答案不提交第三方'),'voice_sensitive_text'],
    ['empty',()=>({finalResponse:''}),async()=>assert.fail('空答案不调用语音接口'),'empty_model_response']
  ]) await t.test(name,async t=>{
    const f=await fixture(t,run,{speech});
    const result=await f.post('/turn',{...input,text:'请用语音回答我，讲个故事'});
    assert.notEqual(result.code,200);
    assert.equal(result.error,expected);
    assert.equal(result.text,undefined);
    assert.equal(f.store.history('123:group:456').filter(m=>m.role==='assistant').length,0);
    const replay=await f.post('/turn',{...input,text:'请用语音回答我，讲个故事'});
    assert.equal(replay.error,'previous_execution_unconfirmed');
    assert.equal(f.calls.length,1);
  });
});

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
