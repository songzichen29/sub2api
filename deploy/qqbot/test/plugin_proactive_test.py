"""Exercise the actual AstrBot adapter methods with an in-memory OneBot client."""
import importlib.util
import pathlib
import sys
import types
import unittest
from unittest.mock import AsyncMock, Mock, patch


def load_bridge():
    root = pathlib.Path(__file__).resolve().parents[1] / 'astrbot_plugin'
    package = types.ModuleType('hotgroup_test_plugin')
    package.__path__ = [str(root)]
    event = types.ModuleType('astrbot.api.event')
    event.AstrMessageEvent = object
    event.MessageChain = Mock
    event.filter = types.SimpleNamespace(event_message_type=lambda *a, **k: lambda fn: fn,
                                        EventMessageType=types.SimpleNamespace(ALL=1))
    star = types.ModuleType('astrbot.api.star')
    star.Context = object
    star.Star = object
    star.register = lambda *args: lambda cls: cls
    api = types.ModuleType('astrbot.api')
    api.logger = Mock()
    modules = {'hotgroup_test_plugin': package, 'aiohttp': Mock(), 'astrbot': types.ModuleType('astrbot'),
               'astrbot.api': api, 'astrbot.api.event': event, 'astrbot.api.star': star}
    with patch.dict(sys.modules, modules):
        spec = importlib.util.spec_from_file_location('hotgroup_test_plugin.main', root / 'main.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
    return module.DSHBridge


Bridge = load_bridge()


class ProactiveTests(unittest.IsolatedAsyncioTestCase):
    def bridge(self, receipt=None):
        instance = object.__new__(Bridge)
        instance.cfg = {'enabled': True, 'proactive_enabled': True, 'bot_id': '123'}
        instance.policy = AsyncMock()
        instance.last_reply, instance.locks = {}, {}
        bot = types.SimpleNamespace(api=types.SimpleNamespace(call_action=AsyncMock(side_effect=[{'user_id': 123}, receipt or {}])))
        platform = types.SimpleNamespace(get_client=lambda: bot)
        instance.context = types.SimpleNamespace(get_platform=lambda name: platform)
        instance.call = AsyncMock(side_effect=[{'id': 'moment', 'chat': '456', 'text': '午饭吃什么？'}, {'claimed': True}, {'ok': True}])
        return instance, bot

    async def test_no_incoming_event_needed_and_real_message_receipt_is_reported(self):
        bridge, bot = self.bridge({'message_id': -4567})
        await bridge.proactive_tick()
        self.assertEqual(bridge.call.await_args_list[0].args, ('/proactive/next', {}))
        self.assertEqual(bot.api.call_action.await_args_list[-1].args, ('send_group_msg',))
        self.assertEqual(bot.api.call_action.await_args_list[-1].kwargs['group_id'], 456)
        self.assertEqual(bridge.call.await_args_list[-1].args[1], {'id': 'moment', 'status': 'sent', 'message_ids': ['-4567']})

    async def test_missing_receipt_is_unconfirmed_and_never_retried(self):
        bridge, bot = self.bridge()
        with self.assertRaises(RuntimeError):
            await bridge.proactive_tick()
        self.assertEqual(bot.api.call_action.await_count, 2)
        self.assertEqual(bridge.call.await_args_list[-1].args[1]['status'], 'unconfirmed')

    async def test_wrong_logged_in_qq_never_generates_a_topic(self):
        bridge, bot = self.bridge()
        bot.api.call_action.side_effect = [{'user_id': 999}]
        await bridge.proactive_tick()
        bridge.call.assert_not_awaited()

    async def test_disabled_configuration_and_cancelled_claim_do_not_send(self):
        bridge, bot = self.bridge()
        bridge.cfg['proactive_enabled'] = False
        await bridge.proactive_tick()
        bot.api.call_action.assert_not_awaited()
        bridge.cfg['proactive_enabled'] = True
        bridge.call.side_effect = [{'id': 'moment', 'chat': '456', 'text': '午饭吃什么？'}, {'claimed': False}]
        await bridge.proactive_tick()
        self.assertEqual(bot.api.call_action.await_count, 1)


if __name__ == '__main__':
    unittest.main()
