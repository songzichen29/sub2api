import asyncio
import base64
import json
import random
import time
from contextlib import asynccontextmanager
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo

import aiohttp
from astrbot.api.event import AstrMessageEvent, MessageChain, filter
from astrbot.api.star import Context, Star, register
from astrbot.api import logger
from .media_helpers import decode_image_base64, image_url, image_mime, image_segments, is_sticker, requests_tool, requests_collection, requests_image_edit, message_packets, receipt_id, should_load_images


class BridgeError(RuntimeError):
    pass


class PartialDelivery(RuntimeError):
    def __init__(self, receipts):
        super().__init__("QQ_delivery_unconfirmed")
        self.receipts = receipts


@register("sub2api_dsh_bridge", "songzichen29", "QQ DSH 会话桥", "0.1.0")
class DSHBridge(Star):
    def __init__(self, context: Context):
        super().__init__(context)
        self.cfg = json.loads(Path("/run/qqbot/config.json").read_text(encoding="utf-8"))
        self.token = Path("/run/secrets/bridge-token").read_text().strip()
        self.http = None
        self.last_reply = {}
        self.locks = {}
        self.pending = {}
        self.proactive_task = None

    async def initialize(self):
        self.http = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=235))
        # Always poll so enabling hot-group moments in the site takes effect
        # without a plugin restart or a new incoming QQ message.
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
                raise BridgeError(value.get("message") or value.get("error", "bridge_rejected"))
            return value

    async def images(self, raw, event):
        output = []
        segments = image_segments(raw)

        async def download(url):
            if not url:
                return None
            async with self.http.get(url, allow_redirects=False, timeout=aiohttp.ClientTimeout(total=15)) as response:
                if response.status != 200:
                    return None
                data = bytearray()
                async for chunk in response.content.iter_chunked(65536):
                    data.extend(chunk)
                    if len(data) > 6 * 1024 * 1024:
                        raise ValueError("image_too_large")
                return bytes(data)

        total = 0
        for index, segment in enumerate(segments[:4], 1):
            info = segment.get("data", {})
            url = image_url(info.get("url")) or image_url(info.get("file", ""))
            try:
                data = await download(url)
                details = None
                if data is None and info.get("file"):
                    details = await event.bot.api.call_action("get_image", file=info["file"])
                    data = decode_image_base64(details.get("base64"))
                    if data is None:
                        data = await download(image_url(details.get("url", "")))
                if data is None:
                    continue
                mime = image_mime(data)
                if mime and total + len(data) <= 12 * 1024 * 1024:
                    total += len(data)
                    output.append({"mimeType": mime, "data": base64.b64encode(data).decode("ascii"), "source_index": index, "is_meme": is_sticker(segment), "label": str(info.get("summary", ""))[:60]})
            except Exception:
                logger.warning("QQ DSH image attachment could not be loaded")
        return output, len(segments)

    async def quoted(self, raw, event):
        for segment in raw.get("message", []):
            if segment.get("type") != "reply":
                continue
            try:
                quoted = await event.bot.api.call_action("get_msg", message_id=segment["data"]["id"])
                # get_msg adapters may put the sender only inside sender.user_id.
                quoted = {**quoted, "user_id": quoted.get("user_id") or quoted.get("sender", {}).get("user_id")}
                # A quote cannot make us load another group or a private chat.
                if raw.get("message_type") == "group" and str(quoted.get("group_id")) == str(raw.get("group_id")):
                    return quoted
                if raw.get("message_type") == "private" and quoted.get("message_type") == "private" and not quoted.get("group_id"):
                    peer = str(raw.get("user_id"))
                    if str(quoted.get("user_id")) == peer:
                        return quoted
                    if str(quoted.get("user_id")) == self.cfg["bot_id"]:
                        known = await self.call("/references/check", {"bot": self.cfg["bot_id"], "sender": peer, "chat": peer, "group": False,
                            "message_id": str(raw.get("message_id")), "reference_id": str(segment["data"]["id"])})
                        if known.get("known"):
                            return quoted
            except Exception:
                logger.warning("QQ DSH quoted message could not be loaded")
        return None

    async def deliver(self, bot, result, chat, is_group, receipts=None):
        # Keep the caller's list updated even when cancellation interrupts a send.
        receipts = receipts if receipts is not None else []
        try:
            for packet in message_packets(result):
                target = {"group_id": int(chat)} if is_group else {"user_id": int(chat)}
                response = await bot.api.call_action("send_group_msg" if is_group else "send_private_msg", message=packet, **target)
                receipts.append(receipt_id(response))
                if result.get("id"):
                    await self.call("/delivery/receipt", {"id": result["id"], "message_ids": [receipts[-1]]})
        except Exception as error:
            raise PartialDelivery(receipts) from error
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
        received_at = time.time()
        try:
            await asyncio.wait_for(self.queued_message(event, received_at), timeout=225)
        except asyncio.TimeoutError:
            await self.audit_failure(event, "turn_expired")
            await event.send(MessageChain().message("这条消息等待或处理超时，请重新发送。"))
        except BridgeError as error:
            if "排队消息已满" in str(error):
                await self.audit_failure(event, "queue_full")
            await event.send(MessageChain().message(str(error)))

    @asynccontextmanager
    async def conversation_slot(self, key):
        if not hasattr(self, "pending"):
            self.pending = {}
        if self.pending.get(key, 0) >= 32:
            raise BridgeError("本会话排队消息已满，请稍后重新发送。")
        self.pending[key] = self.pending.get(key, 0) + 1
        try:
            async with self.locks.setdefault(key, asyncio.Lock()):
                yield
        finally:
            self.pending[key] -= 1
            if self.pending[key] == 0:
                self.pending.pop(key, None)
                self.locks.pop(key, None)

    async def queued_message(self, event, received_at):
        raw = event.message_obj.raw_message
        if not isinstance(raw, dict):
            try:
                raw = dict(raw)
            except (TypeError, ValueError):
                return
        group = raw.get("message_type") == "group"
        key = ("group:" if group else "private:") + str(raw.get("group_id" if group else "user_id", ""))
        # Acquire before policy/quote/image IO so slow downloads cannot reorder
        # messages from the same conversation.
        async with self.conversation_slot(key):
            await self.handle_message(event, received_at)

    async def audit_skip(self, identity, reason):
        try:
            await asyncio.wait_for(self.call("/events", {**identity, "reason": reason}), timeout=3)
        except Exception:
            logger.warning("QQ DSH skip reason could not be reported: %s", reason)

    async def audit_failure(self, event, reason):
        raw = event.message_obj.raw_message
        if isinstance(raw, dict):
            sender = str(raw.get("user_id", ""))
            group = raw.get("message_type") == "group"
            await self.audit_skip({"bot": str(raw.get("self_id", "")), "sender": sender,
                "chat": str(raw.get("group_id", "")) if group else sender, "group": group,
                "message_id": str(raw.get("message_id", ""))}, reason)

    async def handle_message(self, event, received_at):
        try:
            await self.policy()
        except Exception:
            logger.warning("QQ DSH policy unavailable")
            return
        cfg = dict(self.cfg)
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
        if str(raw.get("self_id")) != cfg["bot_id"] or sender == cfg["bot_id"]:
            return
        if is_group and group not in cfg["groups"]:
            return
        if not is_group and sender not in cfg["private_users"]:
            return
        identity = {"bot": cfg["bot_id"], "sender": sender, "chat": chat, "group": is_group,
                    "message_id": str(raw.get("message_id", event.message_obj.message_id))}
        if not cfg.get("enabled", False) or cfg.get("chat_disabled", False) or any(p.get("group_id") == group and p.get("disable_chat") for p in cfg.get("group_policies", [])):
            await self.audit_skip(identity, "chat_disabled")
            return
        text = "".join(str(s.get("data", {}).get("text", "")) for s in raw.get("message", []) if s.get("type") == "text").strip()
        received_images = image_segments(raw)
        mentioned = any(s.get("type") == "at" and str(s.get("data", {}).get("qq")) == cfg["bot_id"] for s in raw.get("message", []))
        quote = await self.quoted(raw, event)
        direct = not is_group or mentioned or requests_tool(text) or (quote and str(quote.get("user_id")) == cfg["bot_id"]) or text.startswith(("/聊天", "/分身", "/清空")) or text in ("清空", "清空对话") or any(name in text for name in cfg.get("names", ["小碗"]))
        loaded = None
        if cfg.get("collect_memes", False) and any(is_sticker(s) for s in image_segments(raw)):
            loaded, _ = await self.images(raw, event)
            if loaded:
                try:
                    await self.call("/attachments", {"bot": cfg["bot_id"], "sender": sender, "chat": chat, "group": is_group, "message_id": str(raw.get("message_id")), "images": loaded})
                except Exception:
                    logger.warning("QQ DSH sticker collection unavailable")
        now = time.time()
        if not direct:
            if not cfg.get("ambient_enabled", False) or group not in cfg.get("ambient_groups", cfg["groups"]):
                await self.audit_skip(identity, "ambient_disabled")
                return
            hour = datetime.now(ZoneInfo("Asia/Shanghai")).hour
            start, end = cfg.get("active_start_hour", 8), cfg.get("active_end_hour", 2)
            in_hours = (start <= hour < end) if start < end else (hour >= start or hour < end)
            if not in_hours:
                await self.audit_skip(identity, "outside_active_hours")
                return
            if now - self.last_reply.get(chat, 0) < cfg.get("cooldown_seconds", 5):
                await self.audit_skip(identity, "cooldown")
                return
            if random.random() * 100 >= cfg.get("ambient_sample_percent", 100):
                await self.audit_skip(identity, "sampled_out")
                return
        try:
            load_images = should_load_images(cfg.get("image_input_enabled", False), direct, received_images)
            tool_images = (cfg.get("collect_memes", False) and requests_collection(text)) or requests_image_edit(text)
            images, count = (loaded, len(received_images)) if loaded is not None else ([], len(received_images))
            if (load_images or tool_images) and loaded is None:
                images, count = await self.images(raw, event)
            if (load_images or tool_images) and not received_images and quote and image_segments(quote):
                images, count = await self.images(quote, event)
            if not text and not load_images:
                return
            forwarded_images = images if load_images else []
            payload = {
                "bot": cfg["bot_id"], "sender": sender, "chat": chat, "group": is_group,
                "message_id": str(raw.get("message_id", event.message_obj.message_id)),
                "name": event.get_sender_name(), "text": text,
                "role": str(raw.get("sender", {}).get("role", "")),
                "ambient": not direct, "images": forwarded_images, "image_count": max(len(received_images), count),
                "own_images": bool(received_images),
                "image_loaded_count": len(images),
                "received_at": int(received_at * 1000), "deadline_at": int((received_at + 225) * 1000),
                "image_note": ("GIF只能依据所提供画面判断，不得声称理解完整动画。" if any(i["mimeType"] == "image/gif" for i in images) else "") + ("引用内容暂无法读取。" if quote is None and any(s.get("type") == "reply" for s in raw.get("message", [])) else ""),
            }
            if images and not forwarded_images:
                payload["tool_images"] = images
            if count > 4:
                payload["image_note"] += "每次最多处理4张图片，本条其余图片未处理，请分批发送。"
            reply = next((s for s in raw.get("message", []) if s.get("type") == "reply"), None)
            if reply:
                payload["quote"] = {"message_id": str(reply.get("data", {}).get("id", "")), "available": quote is not None,
                    "sender": str((quote or {}).get("user_id", "")),
                    "name": str((quote or {}).get("sender", {}).get("card") or (quote or {}).get("sender", {}).get("nickname") or "")[:80],
                    "text": "".join(str(s.get("data", {}).get("text", "")) for s in (quote or {}).get("message", []) if s.get("type") == "text")[:6000]}
            result = await self.call("/turn", payload)
            answer = result.get("text", "").strip()
            if (not answer or answer == "[跳过]") and not result.get("media"):
                return
            claim = await self.call("/delivery/claim", {"id": result["id"]})
            if not claim.get("claimed"):
                return
            status, receipts = "unconfirmed", []
            try:
                # Full output is split; an ambiguous send is never replayed.
                await self.deliver(event.bot, {**result, "reply_to": payload["message_id"]}, chat, is_group, receipts)
                status = "sent"
                self.last_reply[chat] = time.time()
            except PartialDelivery as error:
                receipts = error.receipts
                status = "partial" if receipts else "unconfirmed"
                raise
            finally:
                if status == "unconfirmed" and receipts:
                    status = "partial"
                await self.call("/delivery/finish", {"id": result["id"], "status": status, "message_ids": receipts})
        except Exception as error:
            logger.warning("QQ DSH bridge turn failed: %s", str(error) if isinstance(error, BridgeError) else type(error).__name__)
            if direct:
                # This notice does not assert that the original message was delivered.
                await event.send(MessageChain().message(str(error) if isinstance(error, BridgeError) else "这次对话暂时没有完成，请稍后再试；站点签到、余额等指令仍走原服务。"))

    async def proactive(self):
        while True:
            await asyncio.sleep(15)
            try:
                await self.proactive_tick()
            except Exception as error:
                logger.warning("QQ DSH hot-group turn failed: %s", type(error).__name__)

    async def proactive_tick(self):
        await self.policy()
        if not self.cfg.get("enabled") or self.cfg.get("chat_disabled") or not self.cfg.get("proactive_enabled"):
            return
        # The adapter is available after restart even without cached origins.
        platform = self.context.get_platform("aiocqhttp")
        if platform is None:
            return
        bot = platform.get_client()
        login = await bot.api.call_action("get_login_info")
        if str(login.get("user_id")) != self.cfg["bot_id"]:
            return
        result = await self.call("/proactive/next", {})
        if not result.get("text") or result["text"].strip() == "[跳过]":
            return
        chat = result["chat"]
        lock = self.locks.setdefault("group:" + chat, asyncio.Lock())
        if lock.locked():
            return
        async with lock:
            claim = await self.call("/delivery/claim", {"id": result["id"]})
            if not claim.get("claimed"):
                return
            status, receipts = "unconfirmed", []
            try:
                await self.deliver(bot, result, chat, True, receipts)
                status = "sent"
                self.last_reply[chat] = time.time()
            except PartialDelivery as error:
                receipts = error.receipts
                status = "partial" if receipts else "unconfirmed"
                raise
            finally:
                if status == "unconfirmed" and receipts:
                    status = "partial"
                await self.call("/delivery/finish", {"id": result["id"], "status": status, "message_ids": receipts})
