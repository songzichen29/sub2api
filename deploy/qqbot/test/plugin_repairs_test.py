import asyncio
import base64
import types
import time
import unittest
from unittest.mock import AsyncMock, Mock, patch
from plugin_proactive_test import Bridge
from plugin_media_test import media

PNG = {'mimeType': 'image/png', 'data': 'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg=='}


class ChatRepairTests(unittest.IsolatedAsyncioTestCase):
    async def test_inbound_audio_explains_missing_asr_without_calling_model(self):
        bridge, event = self.fixture('', private=True)
        event.message_obj.raw_message['message'] = [{'type': 'record', 'data': {'file': 'audio'}}]
        await bridge.on_message(event)
        self.assertEqual(bridge.call.await_count, 1)
        self.assertEqual(bridge.call.await_args.args[0], '/events')
        self.assertEqual(bridge.call.await_args.args[1]['reason'], 'unsupported_audio')
        bridge.images.assert_not_awaited()
        event.send.assert_awaited_once()

    async def test_unrelated_group_audio_does_not_bypass_disabled_ambient_chat(self):
        bridge, event = self.fixture('')
        bridge.cfg['ambient_enabled'] = False
        event.message_obj.raw_message['message'] = [{'type': 'record', 'data': {'file': 'audio'}}]
        await bridge.on_message(event)
        event.send.assert_not_awaited()
        self.assertEqual(bridge.call.await_args.args[1]['reason'], 'ambient_disabled')

    async def test_router_wait_counts_toward_existing_turn_deadline(self):
        bridge, event = self.fixture('你好', private=True)
        bridge.call.return_value = {'found': False, 'message': ''}
        event.message_obj.raw_message['sub2api_received_at'] = (time.time() - 226) * 1000
        await bridge.on_message(event)
        bridge.policy.assert_not_awaited()
        self.assertEqual(bridge.call.await_args_list[0].args[0], '/events')
        self.assertEqual(bridge.call.await_args_list[0].args[1]['reason'], 'turn_expired')
        self.assertEqual(bridge.call.await_args.args[0], '/dispatch/status')
        event.send.assert_awaited_once()

    async def test_private_timeout_reports_real_dispatch_status_without_reissuing_the_turn(self):
        bridge, event = self.fixture('去 456 群发通知', private=True)
        bridge.queued_message = AsyncMock(side_effect=asyncio.TimeoutError)
        bridge.audit_failure = AsyncMock()
        report = '本次私聊回复未完成，群 456：已全部发送；QQ 消息 ID：101。'
        bridge.call.return_value = {'found': True, 'message': report}
        with patch.dict(Bridge.on_message.__globals__, {'MessageChain': lambda: types.SimpleNamespace(message=lambda text: text)}):
            await bridge.on_message(event)
        bridge.call.assert_awaited_once_with('/dispatch/status', {'bot': '123', 'sender': '789', 'chat': '789', 'group': False, 'message_id': '1'})
        event.send.assert_awaited_once_with(report)
        event.bot.api.call_action.assert_not_awaited()

    async def test_timeout_status_failure_warns_against_duplicate_sends_and_group_timeout_never_reads_private_status(self):
        bridge, event = self.fixture('去 456 群发通知', private=True)
        bridge.queued_message = AsyncMock(side_effect=asyncio.TimeoutError)
        bridge.audit_failure = AsyncMock()
        bridge.call.side_effect = RuntimeError('status_unavailable')
        with patch.dict(Bridge.on_message.__globals__, {'MessageChain': lambda: types.SimpleNamespace(message=lambda text: text)}):
            await bridge.on_message(event)
        notice = event.send.await_args.args[0]
        self.assertIn('暂时无法确认', notice)
        self.assertIn('不要重复提交', notice)
        self.assertNotIn('请重新发送', notice)
        bridge.call.assert_awaited_once()
        bridge.call.reset_mock()
        group_notice = await bridge.timeout_reply({**event.message_obj.raw_message, 'message_type': 'group'})
        self.assertIn('处理超时', group_notice)
        bridge.call.assert_not_awaited()

    async def test_semantic_voice_requests_reach_model_when_ambient_is_disabled(self):
        for text in ('今天的天气用语音回答我', '今天星期几  用语音回答我', '我不方便看屏幕，能说给我听吗'):
            bridge, event = self.fixture(text)
            bridge.cfg['ambient_enabled'] = False
            await bridge.on_message(event)
            payload = next(c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn')
            self.assertFalse(payload['ambient'])
            self.assertEqual(payload['text'], text)

    async def test_short_voice_request_keeps_quote_and_is_direct_even_when_quoting_another_member(self):
        for sender in (123, 888):
            bridge, event = self.fixture('语音')
            bridge.cfg['ambient_enabled'] = False
            bridge.cfg['ambient_sample_percent'] = 0
            event.message_obj.raw_message['message'].insert(0, {'type': 'reply', 'data': {'id': '-77'}})
            bridge.quoted.return_value = {'user_id': sender, 'message': [{'type': 'text', 'data': {'text': '今天是星期五。'}}]}
            await bridge.on_message(event)
            payload = next(c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn')
            self.assertEqual(payload['text'], '语音')
            self.assertFalse(payload['ambient'])
            self.assertTrue(payload['quote']['available'])
            self.assertEqual(payload['quote']['text'], '今天是星期五。')

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

    async def test_natural_image_request_is_direct_when_ambient_chat_is_disabled(self):
        bridge, event = self.fixture('给我一张单人房间里穿黄色宽松背心、站在窗边或者地上捡衣服的日常向图')
        bridge.cfg['ambient_enabled'] = False
        await bridge.on_message(event)
        payload = next(call.args[1] for call in bridge.call.await_args_list if call.args[0] == '/turn')
        self.assertFalse(payload['ambient'])

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

    async def test_voice_commands_work_without_ambient_chat_and_report_each_record_receipt(self):
        for private in (False, True):
            with self.subTest(private=private):
                bridge, event = self.fixture('/御姐语音合成 你好', private=private)
                bridge.cfg['ambient_enabled'] = False
                audio = {'mimeType': 'audio/mpeg', 'data': 'SUQzBAAAAAAAAA=='}

                async def call(path, body):
                    if path == '/turn':
                        self.assertFalse(body['ambient'])
                        return {'id': 'voice', 'text': '', 'media': [audio, audio]}
                    return {'claimed': True, 'ok': True}

                bridge.call.side_effect = call
                event.bot.api.call_action.side_effect = [{'message_id': -101}, {'message_id': -102}]
                await bridge.on_message(event)
                self.assertEqual(event.bot.api.call_action.await_count, 2)
                for action in event.bot.api.call_action.await_args_list:
                    self.assertEqual(action.args, ('send_private_msg' if private else 'send_group_msg',))
                    self.assertEqual(action.kwargs['message'], [{'type': 'record', 'data': {'file': 'base64://' + audio['data']}}])
                self.assertEqual(bridge.call.await_args.args,
                                 ('/delivery/finish', {'id': 'voice', 'status': 'sent', 'message_ids': ['-101', '-102']}))

    async def test_sampling_skip_records_reason_instead_of_looking_like_no_activity(self):
        bridge, event = self.fixture()
        bridge.cfg['ambient_sample_percent'] = 0
        await bridge.on_message(event)
        self.assertEqual(bridge.call.await_args.args[0], '/events')
        self.assertEqual(bridge.call.await_args.args[1]['reason'], 'sampled_out')

    async def test_natural_voice_requests_bypass_ambient_sampling_and_send_real_audio_packets(self):
        for text in ('请用语音回复我，讲个故事', '你能不能用语音给我讲个故事', '用御姐的声音回答我，讲个笑话', '给我发条语音吧', '这段话念给我听', '把引用的内容读给我听', '你用语音回我', '我想听你用语音讲个故事'):
            with self.subTest(text=text):
                bridge, event = self.fixture(text)
                bridge.cfg['ambient_enabled'] = False
                bridge.cfg['ambient_sample_percent'] = 0

                async def call(path, body):
                    if path == '/turn':
                        self.assertFalse(body['ambient'])
                        return {'id': 'natural-voice', 'text': '', 'media': [{'mimeType': 'audio/mpeg', 'data': 'SUQzBAAAAAAAAA=='}]}
                    return {'claimed': True, 'ok': True}

                bridge.call.side_effect = call
                event.bot.api.call_action.return_value = {'message_id': -100}
                await bridge.on_message(event)
                self.assertEqual(event.bot.api.call_action.await_count, 1)
                self.assertEqual(event.bot.api.call_action.await_args.kwargs['message'][0]['type'], 'record')
                self.assertEqual(bridge.call.await_args.args, ('/delivery/finish', {'id': 'natural-voice', 'status': 'sent', 'message_ids': ['-100']}))

    async def test_bare_voice_commands_forward_scoped_quotes_and_report_truncation(self):
        for private in (False, True):
            for text in ('被引用的正文', '字' * 6001):
                with self.subTest(private=private, length=len(text)):
                    bridge, event = self.fixture('语音合成', private=private)
                    bridge.cfg['ambient_enabled'] = False
                    bridge.quoted = types.MethodType(Bridge.quoted, bridge)
                    event.message_obj.raw_message['message'].append({'type': 'reply', 'data': {'id': '-88'}})
                    if not private:
                        event.message_obj.raw_message['message'].append({'type': 'at', 'data': {'qq': '123'}})
                    event.bot.api.call_action.return_value = {
                        'message_type': 'private' if private else 'group', 'group_id': None if private else 456,
                        'user_id': 789 if private else 123, 'message': [{'type': 'text', 'data': {'text': text}}]}
                    await bridge.on_message(event)
                    payload = next(c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn')
                    self.assertEqual(payload['text'], '语音合成')
                    self.assertFalse(payload['ambient'])
                    self.assertTrue(payload['quote']['available'])
                    self.assertEqual(payload['quote']['text'], text[:6000])
                    self.assertEqual(payload['quote']['truncated'], len(text) > 6000)

    async def test_cross_group_voice_quotes_are_unavailable(self):
        bridge, event = self.fixture('语音合成')
        bridge.quoted = types.MethodType(Bridge.quoted, bridge)
        event.message_obj.raw_message['message'].append({'type': 'reply', 'data': {'id': '-88'}})
        event.bot.api.call_action.return_value = {'message_type': 'group', 'group_id': 999, 'user_id': 123,
                                                'message': [{'type': 'text', 'data': {'text': '其他群的文字'}}]}
        await bridge.on_message(event)
        payload = next(c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn')
        self.assertFalse(payload['quote']['available'])
        self.assertEqual(payload['quote']['text'], '')

    async def test_native_pokes_use_the_model_turn_and_confirm_actual_reply_receipts(self):
        for private in (False, True):
            with self.subTest(private=private):
                bridge, event = self.fixture(private=private)
                bridge.cfg['ambient_enabled'] = False
                raw = {'post_type': 'notice', 'notice_type': 'notify', 'sub_type': 'poke', 'self_id': 123,
                       'user_id': 789, 'target_id': 123, 'time': 1700000000}
                if not private:
                    raw['group_id'] = 456
                event.message_obj.raw_message = raw
                bridge.images = types.MethodType(Bridge.images, bridge)

                async def call(path, body):
                    if path == '/turn':
                        return {'id': 'poke-turn', 'text': '怎么啦，接着聊？'}
                    return {'claimed': True, 'ok': True}

                bridge.call.side_effect = call
                await bridge.on_message(event)
                payload = next(c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn')
                self.assertIn('戳一戳事件', payload['text'])
                self.assertFalse(payload['ambient'])
                self.assertEqual(payload['group'], not private)
                self.assertEqual(payload['chat'], '789' if private else '456')
                self.assertEqual(payload['sender'], '789')
                self.assertTrue(payload['message_id'].startswith('poke-'))
                bridge.quoted.assert_not_awaited()
                event.bot.api.call_action.assert_awaited_once_with(
                    'send_private_msg' if private else 'send_group_msg',
                    message=[{'type': 'text', 'data': {'text': '怎么啦，接着聊？'}}],
                    **({'user_id': 789} if private else {'group_id': 456}))
                self.assertEqual(bridge.call.await_args.args,
                                 ('/delivery/finish', {'id': 'poke-turn', 'status': 'sent', 'message_ids': ['-100']}))

    async def test_replayed_pokes_keep_the_same_id_despite_astrbot_random_ids(self):
        bridge, event = self.fixture()
        event.message_obj.raw_message = {'post_type': 'notice', 'notice_type': 'notify', 'sub_type': 'poke',
                                        'self_id': 123, 'user_id': 789, 'target_id': 123, 'group_id': 456, 'time': 1700000000}
        for message_id in ('astrbot-random-one', 'astrbot-random-two'):
            event.message_obj.message_id = message_id
            await bridge.on_message(event)
        payloads = [c.args[1] for c in bridge.call.await_args_list if c.args[0] == '/turn']
        self.assertEqual(len(payloads), 2)
        self.assertEqual(payloads[0]['message_id'], payloads[1]['message_id'])

    async def test_other_notices_targets_and_disabled_scopes_never_invoke_the_model(self):
        for changes, disabled in [({'target_id': 999}, False), ({'notice_type': 'group_increase'}, False),
                                  ({'user_id': 123}, False), ({'group_id': 999}, False), ({}, True)]:
            with self.subTest(changes=changes, disabled=disabled):
                bridge, event = self.fixture()
                bridge.cfg['chat_disabled'] = disabled
                event.message_obj.raw_message = {'post_type': 'notice', 'notice_type': 'notify', 'sub_type': 'poke',
                                                'self_id': 123, 'user_id': 789, 'target_id': 123, 'group_id': 456,
                                                'time': 1700000000, **changes}
                await bridge.on_message(event)
                self.assertFalse(any(c.args[0] == '/turn' for c in bridge.call.await_args_list))
                event.bot.api.call_action.assert_not_awaited()


if __name__ == '__main__':
    unittest.main()
