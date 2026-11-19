// 音色 ID 来自用户指定的 ApiPost 文档；多 ID 音色固定使用文档列出的第一个。
const voices = {
  可莉: 'a54b3b693c6a4218bcd758a5b8b1fceb',
  塞恩: 'f5e0b2a2403a4ab7b561c38b8cccfe12',
  猫咪: '8ef2535f81dd4820bc1118943d4f9db1',
  妮蔻: '1aa39c8196a349cfb459ef6e0eb2c7bb',
  Nico: '1aa39c8196a349cfb459ef6e0eb2c7bb',
  盖伦: '91a4195171884c999579109828cf5664',
  御姐: '8a88ef7d77d748e59879d8994e216e01',
  刘亦菲: '88c129ca61fe4629b4ed8736e78eff42',
  蒋介石: '918a8277663d476b95e2c4867da0f6a6',
  军事: '2cb86560d1a541fa842a8571a5ffa70b',
  曼波: '561fcedfdf0e4e1399d1bc4930d50c0e',
  贝利亚: '99503144194c45ed8fb998ceac181dcc',
  蔡徐坤: 'e4642e5edccd4d9ab61a69e82d4f8a14',
  麦克阿瑟: '405736979e244634914add64e37290b0',
  影视解说: 'b4bdf5dc66004241a21ff2df165bf442',
  宣传片: 'dd43b30d04d9446a94ebe41f301229b5',
  曹操: 'f6f293aabfe24e46aff0fc309c233d31',
  乌萨奇: '8c66303b8bd14179a564974ceeb7f571',
  卢本伟: '5a918b41b2034f9b81a8da9cb08883cf',
  泽拉斯: '819d6051dc9b4cd6b0150b4e74d3f034',
  派克: '25643b27e8694ebabdd309cb6735b59a',
  雷军: 'aebaa2305aa2452fbdc8f41eec852a79',
  爱莉希雅: 'f06ed9ea97004b45ae790daf61a7f4c0',
  空之律者: '19553e11d45046d5ba09e232143e6549',
  典韦: '2d929fdc119346cdb9e03f614b2fd9d4',
  鳄鱼: '3b6f2ec364834090bb404cba87c19121',
  李云龙: '2e576989a8f94e888bf218de90f8c19a'
};
const commandPattern = new RegExp(`^/?(${Object.keys(voices).join('|')})?语音合成(?=\\s|[:：]|$)\\s*[:：]?\\s*([\\s\\S]*)$`, 'u');
const requestPrefix = '(?:(?:我想|我希望)(?:让你|听你|你)\\s*)?(?:(?:请|麻烦|劳驾)(?:你)?\\s*)?(?:你\\s*)?(?:(?:能不能|可不可以|可以|能)\\s*)?(?:(?:帮|给|替)我\\s*)?';
const voiceNames = Object.keys(voices).join('|');
const readTarget = '(?:这段话|这句话|这段文字|这条消息|引用(?:的)?(?:消息|内容|文字)?|上面(?:的)?(?:消息|内容|文字))';
const readingPattern = new RegExp(`^${requestPrefix}(?:用(${voiceNames})(?:的)?(?:声音|音色|语音)?\\s*)?(?:把|将)?${readTarget}?(?:朗读|念|读)(?:一下)?(?:给我听|出来)?(?:一下)?(?:好吗|可以吗|吧|吗)?[。？！!?]?\\s*(?:[:：]\\s*([\\s\\S]*))?$`, 'u');
const quotedReadingPattern = new RegExp(`^${requestPrefix}(?:用(${voiceNames})(?:的)?(?:声音|音色|语音)?\\s*)?(?:把|将)?[“「"]([\\s\\S]+)[”」"](?:朗读|念|读)(?:一下)?(?:给我听|出来)(?:吧|好吗|吗)?[。？！!?]?$`, 'u');
const replyPattern = new RegExp(`^${requestPrefix}(?:(?:用|通过)(?:(${voiceNames})(?:的)?(?:声音|音色|语音)?|语音)(?:来)?(?:给我|跟我|和我)?(?:回复|回答|答复|回我|讲|说|解释|介绍|聊|告诉)|语音(?:回复|回答|答复|回我|讲|说)|(?:给我)?发(?:一)?(?:条|段|个)?语音(?:回复|回答|答复|给我|讲|说|聊|吧|好吗|可以吗|吗|$))`, 'u');
const textReplyPattern = new RegExp(`^${requestPrefix}(?:(?:不要|别|不用|不需要|无需)(?:再)?(?:用|发)?(?:语音|(?:${voiceNames})(?:的)?(?:声音|音色))|(?:还是|改成|改用)?(?:只)?(?:用|发)?(?:文字|文本)(?:回复|回答|答复|就好|就行|吧|$))`, 'u');

export function parseVoiceCommand(text) {
  const match = commandPattern.exec(text);
  if (!match) return;
  const voice = match[1];
  return { voice: voice || '默认', referenceId: voice ? voices[voice] : 'fbe02f8306fc4d3d915e9871722a39d5', text: match[2].trim() };
}

export function parseVoiceRequest(text) {
  const command = parseVoiceCommand(text);
  if (command) return { ...command, mode: 'read' };
  const reading = quotedReadingPattern.exec(text) || readingPattern.exec(text);
  if (reading) return { ...parseVoiceCommand((reading[1] || '') + '语音合成'), text: (reading[2] || '').trim(), mode: 'read' };
  let request;
  // 只读当前消息中的直接请求；后面的“改用文字”等明确指令覆盖前面的语音要求。
  for (const clause of text.split(/[，,。！!？?；;\n]/u)) {
    const directive = clause.trim();
    if (textReplyPattern.test(directive)) { request = undefined; continue; }
    const reply = replyPattern.exec(directive);
    if (reply) request = { ...parseVoiceCommand((reply[1] || '') + '语音合成'), mode: 'reply' };
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
