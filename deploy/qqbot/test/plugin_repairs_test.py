import asyncio
import base64
import types
import unittest
from unittest.mock import AsyncMock, Mock
from plugin_proactive_test import Bridge
from plugin_media_test import media

PNG = {'mimeType': 'image/png', 'data': 'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg=='}


class ChatRepairTests(unittest.IsolatedAsyncioTestCase):
    def fixture(self, text='你好', message_id=1, images=None, private=False):
        bridge = object.__new__(Bridge)
        bridge.cfg = {'enabled': True, 'bot_id': '123', 'groups': ['456'], 'private_users': ['789'], 'names': ['小碗'],
                      'ambient_enabled': True, 'active_start_hour': 0, 'active_end_hour': 0, 'cooldown_seconds': 0,
                      'ambient_sample_percent': 100, 'collect_memes': True, 'image_input_enabled': True}
        bridge.locks, bridge.pending, bridge.last_reply = {}, {}, {}
        bridge.policy, bridge.quoted = AsyncMock(), AsyncMock(return_value=None)
        bridge.images = AsyncMock(return_value=([PNG], 1))
        bridge.call = AsyncMock(return_value={'text': '[跳过]'})
        raw = {'self_id': 123, 'user_id': 789, 'group_id': 456, 'message_type': 'private' if private else 'group', 'message_id': message_id,
               'message': [{'type': 'text', 'data': {'text': text}}, *(images or [])]}
        event = Mock()
        event.message_obj = types.SimpleNamespace(raw_message=raw, message_id=message_id)
        event.get_sender_id.return_value, event.get_sender_name.return_value = '789', '成员'
        event.send = AsyncMock()
        event.bot = types.SimpleNamespace(api=types.SimpleNamespace(call_action=AsyncMock(return_value={'message_id': -100})))
        return bridge, event

    async def test_ambient_messages_queue_instead_of_being_silently_dropped(self):
        bridge, first = self.fixture()
        _, second = self.fixture(message_id=2)
        entered, release = asyncio.Event(), asyncio.Event()
        ids = []

        async def call(path, body):
            if path == '/turn':
                ids.append(body['message_id'])
                if len(ids) == 1:
                    entered.set()
                    await release.wait()
            return {'text': '[跳过]'}

        bridge.call.side_effect = call
        a = asyncio.create_task(bridge.on_message(first))
        await entered.wait()
        b = asyncio.create_task(bridge.on_message(second))
        for _ in range(10):
            await asyncio.sleep(0)
            if bridge.pending.get('group:456') == 2:
                break
        self.assertEqual(bridge.pending.get('group:456'), 2)
        self.assertEqual(ids, ['1'])
        release.set()
        await asyncio.gather(a, b)
        self.assertEqual(ids, ['1', '2'])
        self.assertEqual(bridge.pending, {})
        first.stop_event.assert_called_once()

    async def test_collection_with_vision_off_loads_photo_for_tools_and_preserves_quote(self):
        bridge, event = self.fixture('收藏这张图片')
        bridge.cfg['image_input_enabled'] = False
        event.message_obj.raw_message['message'].append({'type': 'reply', 'data': {'id': '-88'}})
        bridge.quoted.return_value = {'user_id': 888, 'sender': {'nickname': '原作者'}, 'message': [
            {'type': 'text', 'data': {'text': '这是我的猫'}}, {'type': 'image', 'data': {'file': 'photo'}}]}
        await bridge.on_message(event)
        payload = next(c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn')
        self.assertEqual(payload['images'], [])
        self.assertEqual(payload['tool_images'], [PNG])
        self.assertEqual(payload['quote']['sender'], '888')
        self.assertEqual(payload['quote']['text'], '这是我的猫')
        self.assertFalse(payload['own_images'])
        bridge.images.assert_awaited()

    async def test_media_only_and_split_text_delivery_do_not_add_quotes(self):
        bridge, event = self.fixture('发一个表情包')

        async def call(path, body):
            if path == '/turn':
                return {'id': 'turn', 'text': '', 'media': [PNG]}
            return {'claimed': True, 'ok': True}

        bridge.call.side_effect = call
        await bridge.on_message(event)
        packet = event.bot.api.call_action.await_args.kwargs['message']
        self.assertEqual([p['type'] for p in packet], ['image'])
        self.assertEqual(bridge.call.await_args.args, ('/delivery/finish', {'id': 'turn', 'status': 'sent', 'message_ids': ['-100']}))
        packets = media.message_packets({'text': '字' * 2500, 'reply_to': '-44'})
        self.assertEqual(len(packets), 3)
        self.assertFalse(any(s['type'] == 'reply' for p in packets for s in p))
        self.assertEqual(''.join(s['data']['text'] for p in packets for s in p), '字' * 2500)

    async def test_group_and_private_chat_send_plain_text_without_quoting(self):
        for private in (False, True):
            with self.subTest(private=private):
                bridge, event = self.fixture('小碗你好', private=private)

                async def call(path, body):
                    if path == '/turn':
                        return {'id': 'turn', 'text': '你好呀', 'reply_to': '1'}
                    return {'claimed': True, 'ok': True}

                bridge.call.side_effect = call
                await bridge.on_message(event)
                target = {'user_id': 789} if private else {'group_id': 456}
                event.bot.api.call_action.assert_awaited_once_with(
                    'send_private_msg' if private else 'send_group_msg',
                    message=[{'type': 'text', 'data': {'text': '你好呀'}}], **target)
                self.assertEqual(bridge.call.await_args.args,
                                 ('/delivery/finish', {'id': 'turn', 'status': 'sent', 'message_ids': ['-100']}))

    async def test_partial_send_and_cancellation_keep_confirmed_packet_receipts(self):
        for cancel in (False, True):
            bridge, event = self.fixture('小碗请回答')
            sending = asyncio.Event()

            async def send(*args, **kwargs):
                if event.bot.api.call_action.await_count == 1:
                    return {'message_id': -123}
                sending.set()
                if cancel:
                    await asyncio.Event().wait()
                raise RuntimeError('send unavailable')

            async def call(path, body):
                return {'id': 'turn', 'text': '字' * 1500} if path == '/turn' else {'claimed': True, 'ok': True}

            event.bot.api.call_action.side_effect = send
            bridge.call.side_effect = call
            task = asyncio.create_task(bridge.on_message(event))
            await sending.wait()
            if cancel:
                task.cancel()
                with self.assertRaises(asyncio.CancelledError):
                    await task
            else:
                await task
            calls = bridge.call.await_args_list
            self.assertTrue(any(c.args == ('/delivery/receipt', {'id': 'turn', 'message_ids': ['-123']}) for c in calls))
            self.assertEqual(calls[-1].args, ('/delivery/finish', {'id': 'turn', 'status': 'partial', 'message_ids': ['-123']}))
            self.assertEqual(event.bot.api.call_action.await_count, 2)

    async def test_private_quotes_accept_peer_but_require_scope_receipt_for_bot(self):
        bridge, event = self.fixture(private=True)
        raw = event.message_obj.raw_message
        raw['message'].append({'type': 'reply', 'data': {'id': '-9'}})
        quote = {'message_type': 'private', 'user_id': 789, 'message': []}
        event.bot.api.call_action.return_value = quote
        self.assertEqual(await Bridge.quoted(bridge, raw, event), quote)
        quote['user_id'] = 888
        self.assertIsNone(await Bridge.quoted(bridge, raw, event))
        quote['user_id'] = 123
        bridge.call.return_value = {'known': False}
        self.assertIsNone(await Bridge.quoted(bridge, raw, event))
        bridge.call.return_value = {'known': True}
        self.assertEqual(await Bridge.quoted(bridge, raw, event), quote)

    async def test_four_images_keep_original_indices_after_one_download_failure(self):
        bridge, event = self.fixture()
        raw = event.message_obj.raw_message
        raw['message'] = [{'type': 'image', 'data': {'file': str(i)}} for i in range(5)]
        event.bot.api.call_action.side_effect = [{'base64': PNG['data']}, {}, {'base64': PNG['data']}, {'base64': PNG['data']}]
        images, count = await Bridge.images(bridge, raw, event)
        self.assertEqual(count, 5)
        self.assertEqual([i['source_index'] for i in images], [1, 3, 4])
        self.assertEqual(event.bot.api.call_action.await_count, 4)

    async def test_private_quote_supports_nested_onebot_sender(self):
        bridge, event = self.fixture(private=True)
        raw = event.message_obj.raw_message
        raw['message'].append({'type': 'reply', 'data': {'id': '-9'}})
        event.bot.api.call_action.return_value = {'message_type': 'private', 'sender': {'user_id': 789, 'nickname': '本人'}, 'message': []}
        quoted = await Bridge.quoted(bridge, raw, event)
        self.assertEqual(quoted['user_id'], 789)
        event.bot.api.call_action.return_value['sender']['user_id'] = 999
        self.assertIsNone(await Bridge.quoted(bridge, raw, event))

    async def test_sampling_skip_records_reason_instead_of_looking_like_no_activity(self):
        bridge, event = self.fixture()
        bridge.cfg['ambient_sample_percent'] = 0
        await bridge.on_message(event)
        self.assertEqual(bridge.call.await_args.args[0], '/events')
        self.assertEqual(bridge.call.await_args.args[1]['reason'], 'sampled_out')


if __name__ == '__main__':
    unittest.main()
