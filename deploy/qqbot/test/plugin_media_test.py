import importlib.util
import pathlib
import unittest

path = pathlib.Path(__file__).resolve().parents[1] / 'astrbot_plugin' / 'media_helpers.py'
spec = importlib.util.spec_from_file_location('media_helpers', path)
media = importlib.util.module_from_spec(spec)
spec.loader.exec_module(media)


class PluginMediaTests(unittest.TestCase):
    def test_qq_images_upgrade_and_private_or_untrusted_sources_are_rejected(self):
        self.assertEqual(media.image_url('http://gchat.qpic.cn/test?token=abc'), 'https://gchat.qpic.cn/test?token=abc')
        for value in ('https://qq.com.evil.test/a', 'file:///secret', 'http://127.0.0.1/a', 'https://x:y@qq.com/a', 'https://qpic.cn:8080/a'):
            self.assertIsNone(media.image_url(value))
        self.assertEqual(media.image_mime(b'GIF89a000'), 'image/gif')
        self.assertIsNone(media.image_mime(b'<html>error</html>'))

    def test_onebot_base64_image_payload_is_decoded_and_validated(self):
        png = 'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAIAAAD8GO2jAAAAKElEQVR4nO3NsQ0AAAzCMP5/un0CNkuZ41wybXsHAAAAAAAAAAAAxR4yw/wuPL6QkAAAAABJRU5ErkJggg=='
        self.assertEqual(media.image_mime(media.decode_image_base64(png)), 'image/png')
        self.assertEqual(media.image_mime(media.decode_image_base64('data:image/png;base64,' + png)), 'image/png')
        self.assertIsNone(media.decode_image_base64('not-base64'))
        self.assertIsNone(media.decode_image_base64('aGVsbG8='))

    def test_meme_is_sent_as_image_and_requires_receipt(self):
        packets = media.message_packets({'text': '给你一张', 'media': [{'mimeType':'image/gif', 'data':'R0lGODlhMDAw'}]})
        self.assertEqual(packets[0][1]['type'], 'image')
        self.assertTrue(packets[0][1]['data']['file'].startswith('base64://'))
        self.assertEqual(media.receipt_id({'message_id': -42}), '-42')
        for response in ({}, {'message_id':0}, {'status':'ok','data':{}}, {'message_id':'None'}):
            with self.assertRaises(RuntimeError):
                media.receipt_id(response)

    def test_natural_tool_requests_and_stickers(self):
        for text in ('画一张 疯狂水世界启动的图片', '给我随便发一个表情包', '帮我收集这个表情包', '查一下最新消息', '图片你看不看得懂', '你帮我看下这张图', '这张图是什么意思', '描述一下这张照片'):
            self.assertTrue(media.requests_tool(text))
        self.assertFalse(media.requests_tool('这个表情包真有意思'))
        self.assertTrue(media.is_sticker({'type':'image','data':{'sub_type':1}}))
        self.assertFalse(media.is_sticker({'type':'image','data':{}}))

    def test_voice_packets_use_separate_native_record_messages(self):
        audio = {'mimeType': 'audio/mpeg', 'data': 'SUQzBAAAAAAAAA=='}
        packets = media.message_packets({'text': '语音', 'media': [audio, audio]})
        self.assertEqual([p[0]['type'] for p in packets], ['text', 'record', 'record'])
        self.assertEqual(packets[1], [{'type': 'record', 'data': {'file': 'base64://' + audio['data']}}])
        self.assertEqual(media.message_packets({'text': '', 'media': [audio]}), packets[1:2])
        for command in ('御姐语音合成 你好', '/曼波语音合成：你好', '语音合成 你好', '/语音合成'):
            self.assertTrue(media.requests_tool(command), command)
        self.assertFalse(media.requests_tool('不要御姐语音合成 你好'))

    def test_ambient_messages_with_images_always_load_vision_input(self):
        self.assertTrue(media.should_load_images(True, False, [{'type': 'image'}]))
        self.assertFalse(media.should_load_images(True, False, []))
        self.assertTrue(media.should_load_images(True, True, []))
        self.assertFalse(media.should_load_images(False, False, [{'type': 'image'}]))

    def test_spaced_and_unknown_voice_commands_reach_the_bridge_without_ambient_sampling(self):
        for text in (
            '御姐 语音合成 灰太狼来了', '/曼波\t语音合成：你好', '默认 语音合成 你好',
            '喜羊羊 语音合成 灰太狼来了', '喜羊羊语音合成 灰太狼来了', '/大司马语音合成 你好',
            'voice_2 语音合成 你好',
        ):
            with self.subTest(text=text):
                self.assertTrue(media.requests_tool(text))
        for text in (
            '语音合成是什么', '不要语音合成', '不要御姐语音合成 你好', '什么是语音合成',
            '别用喜羊羊语音合成 你好', '介绍语音合成', '关于语音合成',
            '去A群用喜羊羊语音合成 你好', '以后用喜羊羊语音合成 你好',
        ):
            with self.subTest(text=text):
                self.assertFalse(media.requests_tool(text))

    def test_voice_command_shape_excludes_questions_and_target_conversations(self):
        for text in (
            '不能语音合成', '不能语音合成 你好', '没法语音合成 你好',
            '有哪些音色可以用于语音合成', '有哪些音色可以用于语音合成 请列出',
            '工作群语音合成 你好', '工作群里语音合成 你好', '工作群内语音合成 你好',
            '工作群中语音合成 你好', '工作频道语音合成 你好', '当前私聊语音合成 你好',
            '当前会话语音合成 你好', '喜羊羊语音合成',
        ):
            with self.subTest(text=text):
                self.assertFalse(media.requests_tool(text))
        for text in (
            '喜羊羊 语音合成 灰太狼来了', '喜羊羊语音合成 灰太狼来了',
            '/大司马语音合成', '未知语音合成 你好', '御姐 语音合成',
        ):
            with self.subTest(text=text):
                self.assertTrue(media.requests_tool(text))

    def test_marketplace_stickers_converted_to_images_are_collected(self):
        for segment in (
            {'type': 'mface', 'data': {'url': 'https://qq.com/sticker.gif'}},
            {'type': 'image', 'data': {'sub_type': 1}},
            {'type': 'image', 'data': {'sub_type': '1'}},
            {'type': 'image', 'data': {'sub_type': 0, 'emoji_id': '123456', 'emoji_package_id': 123}},
            {'type': 'image', 'data': {'emoji_id': 'marketplace-face'}},
        ):
            with self.subTest(segment=segment):
                self.assertTrue(media.is_sticker(segment))

    def test_ordinary_images_and_empty_emoji_metadata_are_not_collected(self):
        for segment in (
            {'type': 'image', 'data': {'url': 'https://qq.com/photo.gif', 'summary': '[动画表情]'}},
            *({'type': 'image', 'data': {'sub_type': 0, 'emoji_id': value}} for value in (None, '', 0, '0')),
            {'type': 'video', 'data': {'sub_type': 1}},
        ):
            with self.subTest(segment=segment):
                self.assertFalse(media.is_sticker(segment))


if __name__ == '__main__':
    unittest.main()
