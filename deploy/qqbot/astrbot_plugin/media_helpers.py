import base64
import binascii
import re
from urllib.parse import urlparse, urlunparse


def image_url(value):
    if not isinstance(value, str):
        return None
    parsed = urlparse(value)
    host = parsed.hostname or ""
    try:
        valid_port = parsed.port in (None, 80, 443)
    except ValueError:
        return None
    if parsed.scheme not in ("https", "http") or parsed.username or parsed.password or not valid_port:
        return None
    if not any(host == domain or host.endswith("." + domain) for domain in ("qpic.cn", "qq.com", "qlogo.cn")):
        return None
    return urlunparse(parsed._replace(scheme="https", netloc=host, fragment=""))


def image_mime(data):
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "image/png"
    if data.startswith(b"\xff\xd8\xff"):
        return "image/jpeg"
    if data.startswith((b"GIF87a", b"GIF89a")):
        return "image/gif"
    if data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return "image/webp"
    return None


def decode_image_base64(value):
    """Decode OneBot's optional local-file payload and accept only real images."""
    if not isinstance(value, str):
        return None
    encoded = value.strip()
    if encoded.startswith("data:"):
        _, separator, encoded = encoded.partition(",")
        if not separator:
            return None
    try:
        data = base64.b64decode(encoded, validate=True)
    except (ValueError, binascii.Error):
        return None
    if len(data) > 6 * 1024 * 1024 or image_mime(data) is None:
        return None
    return data


def image_segments(raw):
    return [s for s in raw.get("message", []) if s.get("type") in ("image", "mface")]


def should_load_images(enabled, direct, received_images):
    return bool(enabled and (direct or received_images))


def requests_collection(text):
    return bool(re.search(r"(?:收藏|收集|存下|保存).{0,16}(?:表情|图片|照片|这张|那张)|(?:这张|那张).{0,8}(?:收藏|收集|保存)", text))


def requests_image_edit(text):
    return bool(re.search(r"修图|改图|编辑.{0,12}(?:图|照片)|(?:修改|换|改).{0,16}(?:背景|颜色|图|照片)|(?:图|照片).{0,16}(?:修改|改成|换成)", text))


def is_sticker(segment):
    if segment.get("type") == "mface":
        return True
    if segment.get("type") != "image":
        return False
    data = segment.get("data", {})
    # NapCat also converts marketplace faces to image segments with emoji_id.
    return str(data.get("sub_type", "")) == "1" or data.get("emoji_id") not in (None, "", 0, "0")


def requests_tool(text):
    voice = re.match(r"^/?(?:可莉|塞恩|猫咪|妮蔻|Nico|盖伦|御姐|刘亦菲|蒋介石|军事|曼波|贝利亚|蔡徐坤|麦克阿瑟|影视解说|宣传片|曹操|乌萨奇|卢本伟|泽拉斯|派克|雷军|爱莉希雅|空之律者|典韦|鳄鱼|李云龙)?语音合成(?=\s|[:：]|$)", text)
    spoken_reply = re.search(r'(?:^|[，,。！!？?；;\n])\s*(?:(?:我想|我希望)(?:让你|听你|你)\s*)?(?:(?:请|麻烦|劳驾)(?:你)?\s*)?(?:你\s*)?(?:(?:能不能|可不可以|可以|能)\s*)?(?:(?:帮|给|替)我\s*)?(?:(?:用|通过).{0,12}(?:语音|声音|音色)(?:来)?(?:给我|跟我|和我)?(?:回复|回答|答复|回我|讲|说|解释|介绍|聊|告诉)|语音(?:回复|回答|答复|回我|讲|说)|(?:给我)?发(?:一)?(?:条|段|个)?语音|(?:用.{0,12}(?:声音|音色|语音)\s*)?(?:把|将)?(?:这段话|这句话|这段文字|这条消息|引用(?:的)?(?:消息|内容|文字)?|上面(?:的)?(?:消息|内容|文字)|[“「"].+[”」"])?(?:朗读|念|读)(?:一下)?(?:给我听|出来|[:：]))', text)
    semantic_voice = re.fullmatch(r"/?(?:语音|发语音|用语音|来条语音)[。！!？?]*", text) or re.search(r"语音.{0,12}(?:回复|回答|讲|说)|(?:不方便|没法|不能).{0,8}(?:看屏幕|看文字)|(?:说|讲|读|念)给我听", text)
    return requests_collection(text) or requests_image_edit(text) or bool(voice or spoken_reply or semantic_voice) or bool(re.search(r"画一张|画个|画只|帮我画|(?:给我|帮我|来|要)(?:一|两|几)?张[\s\S]*(?:图|画|照片)|生成.{0,8}(?:图片|图像|照片)|发.{0,12}表情|(?:看|识别|分析|描述|读).{0,12}(?:图|照片)|(?:图片|照片|这张图|那张图).{0,12}(?:看|懂|识别|分析|描述|读|什么|内容|颜色)|上网|查一下|查下|搜一下|搜下|最新.{0,12}(?:新闻|消息)|实时", text))


def message_packets(result):
    answer = result.get("text", "").strip()
    packets = [[{"type": "text", "data": {"text": answer[i:i + 1200]}}] for i in range(0, len(answer), 1200)]
    for i, image in enumerate(result.get("media", [])):
        data = base64.b64decode(image["data"], validate=True)
        if image["mimeType"].startswith("audio/"):
            packets.append([{"type": "record", "data": {"file": "base64://" + image["data"]}}])
            continue
        if len(data) > 6 * 1024 * 1024 or image_mime(data) != image["mimeType"]:
            raise ValueError("invalid_outgoing_image")
        segment = {"type": "image", "data": {"file": "base64://" + image["data"]}}
        if i == 0 and packets:
            packets[-1].append(segment)
        else:
            packets.append([segment])
    return packets


def receipt_id(value):
    if isinstance(value, dict) and (value.get("retcode", 0) != 0 or value.get("status", "ok") != "ok"):
        raise RuntimeError("QQ_delivery_unconfirmed")
    data = value.get("data", value) if isinstance(value, dict) else {}
    message_id = data.get("message_id") if isinstance(data, dict) else None
    if not re.fullmatch(r"-?\d+", str(message_id)) or str(message_id) == "0":
        raise RuntimeError("QQ_delivery_unconfirmed")
    return str(message_id)
