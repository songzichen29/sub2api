import { boundedText } from './policy.mjs';
import { sensitiveText } from './memory.mjs';
import { chatAllowed } from './chat-policy.mjs';
import { replyVoice, voiceReplyForbidden, voiceCatalog } from './voice.mjs';
import { effectiveReplySettings } from './reply-settings.mjs';

// Capability descriptions come from the live configuration. A language model
// must not turn an old "I have no tools" answer into the current product state.
export function capabilityReply(text,c) {
  const categories=[/生图|画图|生成图片/u,/看图|识图|图片/u,/表情包/u,/联网|上网/u,/余额|用量|抽奖/u,/语音|朗读/u];
  const count=categories.filter(p=>p.test(text)).length;
  const tools=/(?:你|机器人|小碗).{0,16}(?:什么|哪些|几个|有哪些|有什么).{0,8}(?:工具|功能|能力)/u.test(text);
  const question=/^你(?:现在|目前)?(?:能|可以|是否能|是否可以|支持).{0,100}?(?:吗|么)(.*)$/u.exec(text);
  const styleOnly=/^(?:(?:请)?(?:简短|简单|准确|简要)+(?:地)?(?:回答|说|介绍|说明)(?:一下)?|(?:不要|不用|无需)(?:实际)?(?:查询|执行|调用|任何|操作|工具|或|和)+|[？?。！!，,；;\s])*$/u;
  const why=/^你(?:为什么|为啥|怎么)(?:不能|无法|不可以|不支持).{0,40}(?:生图|生成图片|发图|看图|联网|查余额)[？?。！!\s]*$/u.test(text);
  const task=/(?:吗|么|[？?。！!，,])\s*(?:帮我|给我|替我|请|画|生成|发送|发一|查)/u.test(text);
  if(task || (!tools && !(count>=1 && (why || (question && styleOnly.test(question[1])))))) return;
  const enabled=[];
  if(c.image_generation_enabled) enabled.push('自然语言生图和原图编辑（已开放会话）');
  if(c.image_input_enabled) enabled.push('直接识别发来或引用的图片');
  if(c.web_enabled) enabled.push('联网搜索和读取网页');
  if(c.memes_enabled) enabled.push(c.collect_memes?'发送、收藏表情包':'发送表情包');
  enabled.push('按命令合成 QQ 语音（例如“御姐语音合成 你好”）');
  return '当前支持：'+[...enabled,'用自然语言查询你本人绑定账号的余额、今日用量和抽奖次数'].join('；')+'。直接说要做什么即可，实际查询和图片投递以系统结果为准。';
}

export function validateImages(images=[]) {
  if (!Array.isArray(images) || images.length>4) throw new Error('invalid_image');
  let total=0;const indices=new Set();
  return images.map((image,index)=>{
    if (!image || !/^image\/(png|jpeg|webp|gif)$/u.test(image.mimeType) || typeof image.data!=='string' || image.data.length>8*1024*1024 || !/^[A-Za-z0-9+/]+={0,2}$/u.test(image.data)) throw new Error('invalid_image');
    const data=Buffer.from(image.data,'base64');
    total+=data.length;const ordinal=image.source_index ?? index+1;
    if(total>12*1024*1024 || !Number.isInteger(ordinal) || ordinal<1 || ordinal>4 || indices.has(ordinal))throw new Error('invalid_image');
    indices.add(ordinal);
    const detected=data.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10]))?'image/png':
      data.subarray(0,3).equals(Buffer.from([255,216,255]))?'image/jpeg':
      /^GIF8[79]a$/u.test(data.subarray(0,6).toString('ascii'))?'image/gif':
      data.subarray(0,4).toString('ascii')==='RIFF' && data.subarray(8,12).toString('ascii')==='WEBP'?'image/webp':'';
    if(detected!==image.mimeType || data.toString('base64')!==image.data) throw new Error('invalid_image');
    return {...image,data: data.toString('base64')};
  });
}

export async function executeCapability({name,args}, context, {config,store,siteTool,sendGroupMessage}) {
  const c=config.snapshot(), {input,id,scope}=context;
  if (input.proactive) throw Object.assign(new Error('主动热群没有用户授权，不执行账户或图片工具'), {code:'proactive_tool_denied'});
  if(!chatAllowed(c,input)) throw Object.assign(new Error('本会话已停用'), {code:'scope_denied'});
  context.signal?.throwIfAborted();
  if (name === 'send_group_message') {
    if (input.group) throw new Error('跨群发送仅允许白名单用户在私聊中操作');
    return sendGroupMessage(args, context);
  }
  if (name === 'list_voices') return { voices: voiceCatalog };
  if (name === 'set_reply_mode') {
    if (!['text', 'voice'].includes(args.mode)) throw new Error('invalid_reply_mode');
    if (args.mode === 'voice' && voiceReplyForbidden(input.text)) throw new Error('voice_explicitly_declined');
    context.replyVoice = args.mode === 'voice' ? replyVoice(args.voice ?? context.replyVoice?.voice ?? context.replySettings?.voice) : undefined;
    return { status: 'selected', mode: args.mode, notice: '仅设置本条回答方式，尚未生成或发送语音；请继续输出实际答案。' };
  }
  const call=(body,options={})=>siteTool({...body,group_id:input.group?input.chat:'',member_id:input.sender,message_id:input.message_id},{signal:context.signal,...options});
  if (['list_reply_groups', 'get_reply_settings', 'update_reply_settings'].includes(name)) {
    if (input.group) throw new Error('请在已允许的私聊中管理回复设置，群内消息不能修改这些设置');
    if (name !== 'update_reply_settings') return call({ tool: name });
    if (context.replySettingsWriteAttempted) throw new Error('本条消息已提交过设置变更，请查询结果；继续修改请发送新的消息');
    for (const update of args.updates) {
      if (update.voice !== undefined) replyVoice(update.voice);
      if (update.prompt !== undefined && sensitiveText(update.prompt)) throw new Error('人设中不能保存密钥或口令');
    }
    context.replySettingsWriteAttempted = true;
    const saved = await call({ tool: name, version: args.version, updates: args.updates, operation_id: input.message_id + ':reply-settings' });
    try {
      await config.refresh();
    } catch (error) {
      throw new Error('回复设置已保存，但运行时同步失败；不要重复追加设置，请查询当前设置确认。', { cause: error });
    }
    return { ...saved, effective: effectiveReplySettings(config.snapshot(), input), notice: '设置已保存并同步，后续回合使用新设置；当前回答仍按本轮开始时的设置发送。' };
  }
  const selectedImage=index=>{
    if(!Number.isInteger(index) || index<1 || index>4)throw new Error('请选择第1至4张实际加载的图片');
    let available=store.cachedImages(scope,input.message_id);
    const explicit=Boolean(input.quote || input.image_count || input.image_note);
    if(!available.length && !explicit)available=store.recentSenderImages(scope,input.sender,context.imageCutoff);
    const image=available.find((item,n)=>(item.source_index || n+1)===index);
    if(!image)throw new Error('指定图片未读取成功，请重新发送或明确引用原图；不会改用其他成员的图片');
    return image;
  };
  if(name==='generate_image' || name==='edit_image') {
    if(!c.image_generation_enabled || (input.group && c.group_policies?.some(p=>p.group_id===input.chat && p.disable_images))) throw Object.assign(new Error('当前会话的生图功能已关闭'), {code:'image_generation_disabled'});
    if(input.ambient && !/(画|绘|生成|创作|制作|编辑|修改|换|改).{0,25}(图|画|猫|狗|水世界|照片|背景|颜色)|画(?:一|个|张|只)|修图|改图|(?:给我|帮我|来|要)(?:一|两|几)?张[\s\S]*(?:图|画|照片)/u.test(input.text)) throw Object.assign(new Error('当前消息未识别出画图或改图请求，任务未提交；这不是生图权限关闭'), {code:'image_intent_unrecognized'});
    if(context.generated) throw Object.assign(new Error('本条消息已有生图请求，不重复提交'), {code:'image_already_requested'});
    const prompt=boundedText(args.prompt,6000),source=name==='edit_image'?selectedImage(args.image_index):undefined;
    const format=args.output_format ?? 'auto';
    if(!['auto','png','jpeg','webp','gif'].includes(format)) throw new Error('不支持的图片输出格式');
    context.generated=true;
    return call({tool:name,prompt,...(format!=='auto'?{output_format:format}:{}),...(source?{source_image:{mimeType:source.mimeType,data:source.data}}:{})});
  }
  if(name==='web_search' || name==='web_fetch') {
    if(!c.web_enabled) throw Object.assign(new Error('联网查询已关闭'), {code:'web_disabled'});
    const value=boundedText(name==='web_search'?args.query:args.url,name==='web_search'?400:2000);
    if(sensitiveText(value)) throw Object.assign(new Error('不能把私人凭据发往互联网'), {code:'sensitive_tool_input'});
    return call({tool:name,[name==='web_search'?'query':'url']:value});
  }
  if(name==='collect_meme') {
    if(!c.memes_enabled || !c.collect_memes) throw Object.assign(new Error('表情包收藏已关闭'), {code:'meme_collection_disabled'});
    const tags=boundedText(args.tags,80);
    if(sensitiveText(tags)) throw new Error('收藏参数无效');
    const image=selectedImage(args.image_index);
    store.collectMeme(scope,image,tags);return {status:'collected',tags};
  }
  if(name==='send_meme') {
    if(!c.memes_enabled || c.group_policies?.some(p=>p.group_id===input.chat && p.disable_images)) throw new Error('本会话的表情包发送已关闭');
    if(store.media(id).length>=2) throw new Error('本轮最多发送两张表情包');
    const keyword=boundedText(args.keyword,40);
    if(sensitiveText(keyword) || /\d{5,}|https?:|@/u.test(keyword)) throw new Error('表情关键词不能包含账号或私人资料');
    let image=store.findMeme(scope,keyword) || store.cachedMeme(scope,keyword);
    if(!image) {
      const randomRequest=/(?:随便|随机|随意).{0,8}(?:表情|发|来)|(?:发|来).{0,4}(?:任意|随便|随机|一个表情包|个表情包)/u.test(input.text);
      const primary=/^(?:随便|随机|表情包?)$/u.test(keyword)?'开心':keyword;
      const keywords=randomRequest?[primary,'开心','猫猫']: [primary];
      let lastError;
      const signal=context.signal?AbortSignal.any([context.signal,AbortSignal.timeout(60000)]):AbortSignal.timeout(60000);
      const deadline=Date.now()+60000;
      for(const candidate of [...new Set(keywords)].slice(0,3)) {
        if(signal.aborted || Date.now()>=deadline)break;
        try {image=await call({tool:'search_meme',keyword:candidate},{signal,timeoutMs:Math.max(1,deadline-Date.now())});break;}
        catch(error) {lastError=error;}
      }
      if(!image && randomRequest)image=store.findMeme(scope,'随便');
      if(!image) throw lastError || new Error('表情搜索暂时不可用');
      context.signal?.throwIfAborted();
      validateImages([image]);store.cacheMeme(scope,keyword,image);
    }
    context.signal?.throwIfAborted();
    validateImages([image]);store.addMedia(id,image);
    return {status:'prepared',description:image.description || keyword,notice:'图片已准备，由系统随回复发送，不要声称QQ回执已成功，也不要凭标签编造图片具体内容。'};
  }
  throw new Error('unknown_tool');
}
