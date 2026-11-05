import { boundedText } from './policy.mjs';
import { sensitiveText } from './memory.mjs';

// Capability descriptions come from the live configuration. A language model
// must not turn an old "I have no tools" answer into the current product state.
export function capabilityReply(text,c) {
  const categories=[/生图|画图|生成图片/u,/看图|识图|图片/u,/表情包/u,/联网|上网/u,/余额|用量|抽奖/u];
  const count=categories.filter(p=>p.test(text)).length;
  const tools=/(?:你|机器人|小碗).{0,16}(?:什么|哪些|几个|有哪些|有什么).{0,8}(?:工具|功能|能力)/u.test(text);
  const question=/^你(?:现在|目前)?(?:能|可以|是否能|是否可以|支持).{0,100}(?:吗|么)[？?。！!\s]*$/u.test(text);
  if(!tools && !(question && count>=1)) return;
  const enabled=[];
  if(c.image_generation_enabled) enabled.push('自然语言生图（已开放群）');
  if(c.image_input_enabled) enabled.push('直接识别发来或引用的图片');
  if(c.web_enabled) enabled.push('联网搜索和读取网页');
  if(c.memes_enabled) enabled.push(c.collect_memes?'发送、收藏表情包':'发送表情包');
  return '当前支持：'+[...enabled,'用自然语言查询你本人绑定账号的余额、今日用量和抽奖次数'].join('；')+'。直接说要做什么即可，实际查询和图片投递以系统结果为准。';
}

export function validateImages(images=[]) {
  if (!Array.isArray(images) || images.length>2) throw new Error('invalid_image');
  return images.map(image=>{
    if (!image || !/^image\/(png|jpeg|webp|gif)$/u.test(image.mimeType) || typeof image.data!=='string' || image.data.length>8*1024*1024 || !/^[A-Za-z0-9+/]+={0,2}$/u.test(image.data)) throw new Error('invalid_image');
    const data=Buffer.from(image.data,'base64');
    const detected=data.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10]))?'image/png':
      data.subarray(0,3).equals(Buffer.from([255,216,255]))?'image/jpeg':
      /^GIF8[79]a$/u.test(data.subarray(0,6).toString('ascii'))?'image/gif':
      data.subarray(0,4).toString('ascii')==='RIFF' && data.subarray(8,12).toString('ascii')==='WEBP'?'image/webp':'';
    if(detected!==image.mimeType || data.toString('base64')!==image.data) throw new Error('invalid_image');
    return {...image,data: data.toString('base64')};
  });
}

export async function executeCapability({name,args}, context, {config,store,siteTool}) {
  const c=config.snapshot(), {input,id,scope}=context;
  if(!c.enabled || c.chat_disabled || (input.group && (!c.groups.includes(input.chat) || c.group_policies?.some(p=>p.group_id===input.chat && p.disable_chat)))) throw new Error('本会话已停用');
  const call=body=>siteTool({...body,group_id:input.group?input.chat:'',member_id:input.sender,message_id:input.message_id});
  if(name==='generate_image') {
    if(!c.image_generation_enabled || (input.ambient && !/(画|绘|生成|创作|制作).{0,25}(图|画|猫|狗|水世界|照片)|画(?:一|个|张|只)/u.test(input.text))) throw new Error('仅在用户要求创作图片时生图');
    if(context.generated) throw new Error('本条消息已有生图请求，不重复提交');
    context.generated=true;
    return call({tool:name,prompt:boundedText(args.prompt,6000)});
  }
  if(name==='web_search' || name==='web_fetch') {
    if(!c.web_enabled) throw new Error('联网查询已关闭');
    const value=boundedText(name==='web_search'?args.query:args.url,name==='web_search'?400:2000);
    if(sensitiveText(value)) throw new Error('不能把私人凭据发往互联网');
    return call({tool:name,[name==='web_search'?'query':'url']:value});
  }
  if(name==='collect_meme') {
    if(!c.memes_enabled || !c.collect_memes) throw new Error('表情包收藏已关闭');
    const tags=boundedText(args.tags,80);
    if(sensitiveText(tags) || !Number.isInteger(args.image_index) || args.image_index<1 || args.image_index>2) throw new Error('收藏参数无效');
    const images=store.cachedImages(scope,input.message_id);const available=images.length?images:store.cachedImages(scope,undefined,context.imageCutoff);
    const image=available[args.image_index-1];if(!image) throw new Error('还没有可收藏的图片，请把表情包发来或引用它');
    store.collectMeme(scope,image,tags);return {status:'collected',tags};
  }
  if(name==='send_meme') {
    if(!c.memes_enabled || c.group_policies?.some(p=>p.group_id===input.chat && p.disable_images)) throw new Error('本会话的表情包发送已关闭');
    if(store.media(id).length>=2) throw new Error('本轮最多发送两张表情包');
    const keyword=boundedText(args.keyword,40);
    if(sensitiveText(keyword) || /\d{5,}|https?:|@/u.test(keyword)) throw new Error('表情关键词不能包含账号或私人资料');
    const image=store.findMeme(scope,keyword) || await call({tool:'search_meme',keyword:/^(随便|随机|表情包?)$/u.test(keyword)?'开心':keyword});
    validateImages([image]);store.addMedia(id,image);
    return {status:'prepared',description:image.description || keyword,notice:'图片已准备，由系统随回复发送，不要声称QQ回执已成功，也不要凭标签编造图片具体内容。'};
  }
  throw new Error('unknown_tool');
}
