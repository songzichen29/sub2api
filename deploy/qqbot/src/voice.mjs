import voices from './voice-catalog.json' with { type: 'json' };

export const voiceCatalog = Object.entries(voices).map(([name, referenceId]) => ({ name, reference_id: referenceId }));

const commandPattern = new RegExp(`^/?(?:(${Object.keys(voices).join('|')})\\s*)?语音合成(?=\\s|[:：]|$)\\s*[:：]?\\s*([\\s\\S]*)$`, 'u');
const unknownCommandPattern = /^\/?([\p{L}\p{N}_-]+)\s*语音合成(?=\s|[:：]|$)\s*[:：]?\s*([\s\S]*)$/u;
const conversationalVoiceName = /^(?:不|别|没|无|取消|停止|什么|如何|怎么|为何|为什么|请|你|我|他|她|它|给|去|在|把|将|用|帮|能否|能不能|可不可以|是否|以后|一直|默认|介绍|解释|讨论|关于)|(?:音色|声音|可以|用于)|(?:群(?:里|内|中)?|私聊|会话|频道)$/u;
const requestPrefix = '(?:(?:我想|我希望)(?:让你|听你|你)\\s*)?(?:(?:请|麻烦|劳驾)(?:你)?\\s*)?(?:你\\s*)?(?:(?:能不能|可不可以|可以|能)\\s*)?(?:(?:帮|给|替)我\\s*)?';
const voiceNames = Object.keys(voices).join('|');
const readTarget = '(?:这段话|这句话|这段文字|这条消息|引用(?:的)?(?:消息|内容|文字)?|上面(?:的)?(?:消息|内容|文字))';
const readingPattern = new RegExp(`^${requestPrefix}(?:用(${voiceNames})(?:的)?(?:声音|音色|语音)?\\s*)?(?:把|将)?${readTarget}?(?:朗读|念|读)(?:一下)?(?:给我听|出来)?(?:一下)?(?:好吗|可以吗|吧|吗)?[。？！!?]?\\s*(?:[:：]\\s*([\\s\\S]*))?$`, 'u');
const quotedReadingPattern = new RegExp(`^${requestPrefix}(?:用(${voiceNames})(?:的)?(?:声音|音色|语音)?\\s*)?(?:把|将)?[“「"]([\\s\\S]+)[”」"](?:朗读|念|读)(?:一下)?(?:给我听|出来)(?:吧|好吗|吗)?[。？！!?]?$`, 'u');
const replyPattern = new RegExp(`^${requestPrefix}(?:(?:用|通过)(?:(${voiceNames})(?:的)?(?:声音|音色|语音)?|语音)(?:来)?(?:给我|跟我|和我)?(?:回复|回答|答复|回我|讲|说|解释|介绍|聊|告诉)|语音(?:回复|回答|答复|回我|讲|说)(?=我|一下|下|吧|好吗|可以吗|吗|个|一(?:个|段)|这|那|今天|刚才|[：:\\s]|$)|(?:给我)?发(?:一)?(?:条|段|个)?语音(?:回复|回答|答复|给我|讲|说|聊|吧|好吗|可以吗|吗|$))`, 'u');
const textReplyPattern = new RegExp(`^${requestPrefix}(?:(?:不要|别|不用|不需要|无需)(?:再)?(?:用|发)?(?:语音|(?:${voiceNames})(?:的)?(?:声音|音色))|(?:还是|改成|改用)?(?:只)?(?:用|发)?(?:文字|文本)(?:回复|回答|答复|就好|就行|吧|$))`, 'u');

export function parseVoiceCommand(text) {
  const match = commandPattern.exec(text);
  if (match) {
    const voice = match[1] || '默认';
    return { voice, referenceId: voices[voice], text: match[2].trim() };
  }
  const unknown = unknownCommandPattern.exec(text);
  if (!unknown || (!text.startsWith('/') && !unknown[2].trim()) || conversationalVoiceName.test(unknown[1])) return;
  return { voice: unknown[1], text: unknown[2].trim(), error: 'unknown_voice' };
}

export function isVoiceToolInstruction(answer, request) {
  const technicalQuestion = request.split(/[，,。！!？?；;\n]/u).some(clause =>
    !/(?:不要|别|不用|不需要|无需|不是|不想|禁止)/u.test(clause) &&
    /(?:解释|介绍|说明|如何|怎么|怎样|什么是).*set[_\s]reply[_\s]mode|set[_\s]reply[_\s]mode.*(?:如何|怎么|怎样|是什么|什么意思|用法|参数|接口)/iu.test(clause));
  if (technicalQuestion) return false;
  const instruction = answer.replace(/[`*，,。.!！\s]/gu, '');
  return /^(?:我)?(?:将|会|要|现在|需要|应该|请|先)*(?:调用|使用)set_reply_mode(?:工具)?(?:(?:将|把)?(?:mode|模式)?(?:选择|设置为|设为|切换为|改为|设置))?(?:voice|语音)(?:模式)?$/iu.test(instruction)
    || /^set_reply_mode\(\{"mode":"voice"\}\)$/iu.test(instruction);
}

export function voiceReplyForbidden(text) {
  if (parseVoiceRequest(text)?.mode === 'reply') return false;
  return text.split(/[，,。！!？?；;\n]/u).some(clause => textReplyPattern.test(clause.trim()));
}

export function replyVoice(voice = '默认') {
  if (voice !== '默认' && !Object.hasOwn(voices, voice)) throw new Error('unknown_voice');
  return parseVoiceCommand((voice === '默认' ? '' : voice) + '语音合成');
}

export function parseVoiceRequest(text) {
  const command = parseVoiceCommand(text);
  if (command) return { ...command, mode: 'read' };
  const reading = quotedReadingPattern.exec(text) || readingPattern.exec(text);
  if (reading) return { ...parseVoiceCommand((reading[1] || '') + '语音合成'), text: (reading[2] || '').trim(), mode: 'read' };
  let request;
  // 只读当前消息中的直接请求；后面的“改用文字”等明确指令覆盖前面的语音要求。
  for (const clause of text.split(/[，,。！!？?；;\n]/u)) {
    const directive = clause.trim().replace(/^(?:算了|还是|改成|改用|改为)\s*/u, '');
    if (textReplyPattern.test(directive)) { request = undefined; continue; }
    const reply = replyPattern.exec(directive);
    if (reply && /^(?:是(?:什么|啥)|什么意思)/u.test(directive.slice(reply[0].length))) continue;
    if (reply) request = { ...parseVoiceCommand((reply[1] || '') + '语音合成'), mode: 'reply', voiceSpecified: Boolean(reply[1]) };
  }
  return request;
}

export async function synthesizeVoice(command, { signal, fetcher = fetch } = {}) {
  if (!command.text) throw new Error('voice_text_missing');
  const parts = [];
  let part = '';
  // 接口每次最多 500 字符，按 UTF-16 长度分片且不拆开 emoji 的代理对。
  for (const character of command.text) {
    if (part.length + character.length > 500) {
      parts.push(part);
      part = '';
    }
    part += character;
  }
  parts.push(part);
  const media = [];
  for (const text of parts) {
    const response = await fetcher('https://video.chrelyonly.cn/video-api/getVideo', {
      method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'audio/*, application/octet-stream' },
      body: JSON.stringify({ text, referenceId: command.referenceId }), signal, redirect: 'error'
    });
    if (!response.ok) {
      await response.body.cancel();
      throw new Error('voice_request_failed');
    }
    const chunks = [];
    let size = 0;
    for await (const chunk of response.body) {
      size += chunk.length;
      // 与现有媒体投递大小一致，避免 Base64 超过 OneBot WebSocket 的载荷限制。
      if (size > 6 * 1024 * 1024) throw new Error('voice_response_too_large');
      chunks.push(chunk);
    }
    const data = Buffer.concat(chunks);
    const isWav = data.subarray(0, 4).toString('ascii') === 'RIFF' && data.subarray(8, 12).toString('ascii') === 'WAVE';
    const isMp3 = data.subarray(0, 3).toString('ascii') === 'ID3' || (data[0] === 0xff && (data[1] & 0xe0) === 0xe0);
    // 文档没有响应结构：只接收可识别的音频，JSON/HTML 错误不得当成语音发送。
    if (!isWav && !isMp3) throw new Error('voice_response_invalid');
    media.push({ mimeType: isWav ? 'audio/wav' : 'audio/mpeg', data: data.toString('base64'), description: command.voice + '语音合成' });
  }
  return media;
}
