import asyncio
import base64
import json
import random
import time
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo

import aiohttp
from astrbot.api.event import AstrMessageEvent, MessageChain, filter
from astrbot.api.star import Context, Star, register
from astrbot.api import logger
from .media_helpers import image_url, image_mime, image_segments, is_sticker, requests_tool, message_packets, receipt_id, should_load_images


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
        self.http = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=235))
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
            self.http = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=235))
        async with self.http.post(
            "http://bridge:8787" + path, json=body,
            headers={"x-bridge-token": self.token},
        ) as response:
            value = await response.json()
            if response.status != 200:
                raise RuntimeError(value.get("error", "bridge_rejected"))
            return value

    async def images(self, raw, event):
        output = []
        segments = image_segments(raw)
        for segment in segments[:2]:
            info = segment.get("data", {})
            url = image_url(info.get("url") or info.get("file", ""))
            try:
                if not url and info.get("file"):
                    details = await event.bot.api.call_action("get_image", file=info["file"])
                    url = image_url(details.get("url", ""))
                if not url:
                    continue
                async with self.http.get(url, allow_redirects=False, timeout=aiohttp.ClientTimeout(total=15)) as response:
                    if response.status != 200:
                        continue
                    data = bytearray()
                    async for chunk in response.content.iter_chunked(65536):
                        data.extend(chunk)
                        if len(data) > 6 * 1024 * 1024:
                            raise ValueError("image_too_large")
                    mime = image_mime(data)
                    if mime:
                        output.append({"mimeType": mime, "data": base64.b64encode(data).decode("ascii"), "is_meme": is_sticker(segment), "label": str(info.get("summary", ""))[:60]})
            except Exception:
                logger.warning("QQ DSH image attachment could not be loaded")
        return output, len(segments)

    async def quoted(self, raw, event):
        for segment in raw.get("message", []):
            if segment.get("type") != "reply":
                continue
            try:
                quoted = await event.bot.api.call_action("get_msg", message_id=segment["data"]["id"])
                # A quote cannot make us load another group or a private chat.
                if raw.get("message_type") == "group" and str(quoted.get("group_id")) == str(raw.get("group_id")):
                    return quoted
            except Exception:
                logger.warning("QQ DSH quoted message could not be loaded")
        return None

    async def deliver(self, event, result, chat, is_group):
        receipts = []
        for packet in message_packets(result):
            target = {"group_id": int(chat)} if is_group else {"user_id": int(chat)}
            response = await event.bot.api.call_action("send_group_msg" if is_group else "send_private_msg", message=packet, **target)
            receipts.append(receipt_id(response))
        if not receipts:
            raise RuntimeError("QQ_delivery_unconfirmed")
        return receipts

    async def policy(self):
        if self.http is None:
            self.http = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=235))
        async with self.http.get("http://bridge:8787/policy", headers={"x-bridge-token": self.token}, timeout=aiohttp.ClientTimeout(total=8)) as response:
            if response.status != 200:
                raise RuntimeError("configuration_not_synchronized")
            self.cfg = await response.json()

    @filter.event_message_type(filter.EventMessageType.ALL, priority=100)
    async def on_message(self, event: AstrMessageEvent):
        # Own the event before any await so AstrBot never calls a second model.
        event.should_call_llm(False)
        event.stop_event()
        try:
            await self.policy()
        except Exception:
            logger.warning("QQ DSH policy unavailable")
            return
        if not self.cfg.get("enabled", False) or self.cfg.get("chat_disabled", False):
            return
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
        text = "".join(str(s.get("data", {}).get("text", "")) for s in raw.get("message", []) if s.get("type") == "text").strip()
        received_images = image_segments(raw)
        mentioned = any(s.get("type") == "at" and str(s.get("data", {}).get("qq")) == self.cfg["bot_id"] for s in raw.get("message", []))
        quote = await self.quoted(raw, event)
        direct = not is_group or mentioned or requests_tool(text) or (quote and str(quote.get("user_id")) == self.cfg["bot_id"]) or text.startswith(("/聊天", "/分身", "/清空")) or text in ("清空", "清空对话") or any(name in text for name in self.cfg.get("names", ["小碗"]))
        loaded = None
        if self.cfg.get("collect_memes", False) and any(is_sticker(s) for s in image_segments(raw)):
            loaded, _ = await self.images(raw, event)
            if loaded:
                try:
                    await self.call("/attachments", {"bot": self.cfg["bot_id"], "sender": sender, "chat": chat, "group": is_group, "message_id": str(raw.get("message_id")), "images": loaded})
                except Exception:
                    logger.warning("QQ DSH sticker collection unavailable")
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
                load_images = should_load_images(self.cfg.get("image_input_enabled", False), direct, received_images)
                images, count = (loaded, len(received_images)) if loaded is not None else ([], len(received_images))
                if load_images and loaded is None:
                    images, count = await self.images(raw, event)
                if load_images and not images and quote and image_segments(quote):
                    images, count = await self.images(quote, event)
                if not text and not load_images:
                    return
                forwarded_images = images if load_images else []
                payload = {
                    "bot": self.cfg["bot_id"], "sender": sender, "chat": chat, "group": is_group,
                    "message_id": str(raw.get("message_id", event.message_obj.message_id)),
                    "name": event.get_sender_name(), "text": text,
                    "role": str(raw.get("sender", {}).get("role", "")),
                    "ambient": not direct, "images": forwarded_images, "image_count": max(len(received_images), count),
                    "image_loaded_count": len(images),
                    "image_note": ("GIF只能依据所提供画面判断，不得声称理解完整动画。" if any(i["mimeType"] == "image/gif" for i in images) else "") + ("引用内容暂无法读取。" if quote is None and any(s.get("type") == "reply" for s in raw.get("message", [])) else ""),
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
                    receipts = await self.deliver(event, result, chat, is_group)
                    status = "sent"
                    self.last_reply[chat] = time.time()
                finally:
                    await self.call("/delivery/finish", {"id": result["id"], "status": status, "message_ids": receipts if status == "sent" else []})
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
