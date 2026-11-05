import asyncio
import base64
import json
import random
import time
from datetime import datetime
from pathlib import Path
from urllib.parse import urlparse
from zoneinfo import ZoneInfo

import aiohttp
from astrbot.api.event import AstrMessageEvent, MessageChain, filter
from astrbot.api.star import Context, Star, register
from astrbot.api import logger


@register("sub2api_dsh_bridge", "songzichen29", "QQ DSH 会话桥", "0.1.0")
class DSHBridge(Star):
    def __init__(self, context: Context):
        super().__init__(context)
        self.cfg = json.loads(Path("/run/qqbot/config.json").read_text(encoding="utf-8"))
        self.token = Path("/run/secrets/bridge-token").read_text().strip()
        self.http = None
        self.last_reply = {}
        self.last_seen = {}
        self.origins = {}
        self.locks = {}
        self.proactive_task = None

    async def initialize(self):
        self.http = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=170))
        if self.cfg.get("proactive_enabled", False):
            self.proactive_task = asyncio.create_task(self.proactive())

    async def terminate(self):
        if self.proactive_task:
            self.proactive_task.cancel()
            await asyncio.gather(self.proactive_task, return_exceptions=True)
        if self.http:
            await self.http.close()

    async def call(self, path, body):
        if self.http is None:
            self.http = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=170))
        async with self.http.post(
            "http://bridge:8787" + path, json=body,
            headers={"x-bridge-token": self.token},
        ) as response:
            value = await response.json()
            if response.status != 200:
                raise RuntimeError(value.get("error", "bridge_rejected"))
            return value

    async def images(self, raw):
        output = []
        for segment in raw.get("message", []):
            if segment.get("type") != "image" or len(output) >= 2:
                continue
            url = segment.get("data", {}).get("url", "")
            parsed = urlparse(url)
            host = parsed.hostname or ""
            if parsed.scheme != "https" or not any(host == d or host.endswith("." + d) for d in ("qpic.cn", "qq.com", "qlogo.cn")):
                continue
            try:
                async with self.http.get(url, allow_redirects=False, timeout=aiohttp.ClientTimeout(total=15)) as response:
                    mime = response.headers.get("Content-Type", "").split(";")[0]
                    if response.status != 200 or mime not in ("image/png", "image/jpeg", "image/gif", "image/webp"):
                        continue
                    data = bytearray()
                    async for chunk in response.content.iter_chunked(65536):
                        data.extend(chunk)
                        if len(data) > 6 * 1024 * 1024:
                            raise ValueError("image_too_large")
                    output.append({"mimeType": mime, "data": base64.b64encode(data).decode("ascii")})
            except (aiohttp.ClientError, asyncio.TimeoutError, ValueError):
                logger.warning("QQ DSH image attachment could not be loaded")
        return output

    @filter.event_message_type(filter.EventMessageType.ALL, priority=100)
    async def on_message(self, event: AstrMessageEvent):
        # Own the event before any await so AstrBot never calls a second model.
        event.should_call_llm(False)
        event.stop_event()
        raw = event.message_obj.raw_message
        if not isinstance(raw, dict):
            try:
                raw = dict(raw)
            except (TypeError, ValueError):
                return
        sender = str(raw.get("user_id", event.get_sender_id()))
        group = str(raw.get("group_id", ""))
        is_group = raw.get("message_type") == "group"
        chat = group if is_group else sender
        if str(raw.get("self_id")) != self.cfg["bot_id"] or sender == self.cfg["bot_id"]:
            return
        if is_group and group not in self.cfg["groups"]:
            return
        if not is_group and sender not in self.cfg["private_users"]:
            return
        text = event.message_str.strip()
        mentioned = any(s.get("type") == "at" and str(s.get("data", {}).get("qq")) == self.cfg["bot_id"] for s in raw.get("message", []))
        direct = not is_group or mentioned or text.startswith(("/聊天", "/分身", "/清空")) or text in ("清空", "清空对话") or any(name in text for name in self.cfg.get("names", ["小碗"]))
        now = time.time()
        self.last_seen[chat] = now
        self.origins[chat] = (event.unified_msg_origin, sender, is_group)
        if not direct:
            if not self.cfg.get("ambient_enabled", False) or group not in self.cfg.get("ambient_groups", self.cfg["groups"]):
                return
            hour = datetime.now(ZoneInfo("Asia/Shanghai")).hour
            start, end = self.cfg.get("active_start_hour", 8), self.cfg.get("active_end_hour", 2)
            in_hours = (start <= hour < end) if start < end else (hour >= start or hour < end)
            if not in_hours or now - self.last_reply.get(chat, 0) < self.cfg.get("cooldown_seconds", 5):
                return
            if random.random() * 100 >= self.cfg.get("ambient_sample_percent", 100):
                return
        lock = self.locks.setdefault(chat, asyncio.Lock())
        # Ambient work never waits behind an older response.
        if lock.locked() and not direct:
            return
        async with lock:
            try:
                load_images = self.cfg.get("image_input_enabled", False) and (direct or random.random() * 100 < self.cfg.get("ambient_media_sample_percent", 10))
                payload = {
                    "bot": self.cfg["bot_id"], "sender": sender, "chat": chat, "group": is_group,
                    "message_id": str(raw.get("message_id", event.message_obj.message_id)),
                    "name": event.get_sender_name(), "text": text,
                    "ambient": not direct, "images": await self.images(raw) if load_images else [],
                }
                result = await self.call("/turn", payload)
                answer = result.get("text", "").strip()
                if not answer or answer == "[跳过]":
                    return
                claim = await self.call("/delivery/claim", {"id": result["id"]})
                if not claim.get("claimed"):
                    return
                status = "unconfirmed"
                try:
                    # Full output is split; an ambiguous send is never replayed.
                    for offset in range(0, len(answer), 1200):
                        await event.send(MessageChain().message(answer[offset:offset + 1200]))
                    status = "sent"
                    self.last_reply[chat] = time.time()
                finally:
                    await self.call("/delivery/finish", {"id": result["id"], "status": status})
            except Exception as error:
                logger.warning("QQ DSH bridge turn failed: %s", type(error).__name__)
                if direct:
                    # This notice does not assert that the original message was delivered.
                    await event.send(MessageChain().message("这次对话暂时没有完成，请稍后再试；站点签到、余额等指令仍走原服务。"))

    async def proactive(self):
        while True:
            await asyncio.sleep(random.randint(20 * 60, 60 * 60))
            hour = datetime.now(ZoneInfo("Asia/Shanghai")).hour
            if 2 <= hour < 8:
                continue
            for chat, (origin, sender, is_group) in list(self.origins.items()):
                # No repeated prompts into an inactive group; owner private chat is separate.
                if is_group and time.time() - self.last_seen.get(chat, 0) > 3600:
                    continue
                if time.time() - self.last_reply.get(chat, 0) < 1200:
                    continue
                try:
                    result = await self.call("/turn", {
                        "bot": self.cfg["bot_id"], "chat": chat, "sender": sender, "group": is_group,
                        "message_id": "proactive-" + str(int(time.time())), "name": "",
                        "ambient": True, "text": "根据本会话上下文决定是否自然开口；没有适合的新话题就输出[跳过]。",
                    })
                    if result.get("text") and result["text"].strip() != "[跳过]":
                        claim = await self.call("/delivery/claim", {"id": result["id"]})
                        if claim.get("claimed"):
                            await self.context.send_message(origin, MessageChain().message(result["text"]))
                            self.last_reply[chat] = time.time()
                            await self.call("/delivery/finish", {"id": result["id"], "status": "sent"})
                except Exception as error:
                    logger.warning("QQ DSH proactive turn failed: %s", type(error).__name__)
