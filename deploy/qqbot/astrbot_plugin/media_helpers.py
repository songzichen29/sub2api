import base64
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


def image_segments(raw):
    return [s for s in raw.get("message", []) if s.get("type") in ("image", "mface")]


def is_sticker(segment):
    return segment.get("type") == "mface" or str(segment.get("data", {}).get("sub_type", "")) == "1"


def requests_tool(text):
    return bool(re.search(r"画一张|画个|画只|帮我画|生成.{0,8}(?:图片|图像|照片)|发.{0,12}表情|(?:收藏|收集|存下).{0,12}(?:表情|图片)|(?:看|识别|分析|描述|读).{0,12}(?:图|照片)|(?:图片|照片|这张图|那张图).{0,12}(?:看|懂|识别|分析|描述|读|什么|内容|颜色)|上网|查一下|查下|搜一下|搜下|最新.{0,12}(?:新闻|消息)|实时", text))


def message_packets(result):
    answer = result.get("text", "").strip()
    packets = [[{"type": "text", "data": {"text": answer[i:i + 1200]}}] for i in range(0, len(answer), 1200)]
    for i, image in enumerate(result.get("media", [])):
        data = base64.b64decode(image["data"], validate=True)
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
