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


if __name__ == '__main__':
    unittest.main()
