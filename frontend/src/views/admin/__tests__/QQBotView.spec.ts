import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import QQBotView from '../QQBotView.vue'

const api = vi.hoisted(() => ({ config: vi.fn(), connection: vi.fn(), records: vi.fn(), save: vi.fn(), test: vi.fn() }))
vi.mock('@/api/qqbot', () => ({ qqbotApi: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
enableAutoUnmount(afterEach)

describe('QQBotView', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    api.config.mockResolvedValue({ version: 1, enabled: false, bot_qq: '123456', allowed_groups: ['987654'], access_token: '', access_token_set: true, api_key: '', api_key_set: true, checkin_reward: 0, checkin_reward_max: 0, chat_enabled: false, chat_model: 'model', system_prompt: '', max_output_tokens: 1024, history_messages: 10, daily_chat_limit: 20, total_daily_chat_limit: 200, cooldown_seconds: 5 })
    api.connection.mockResolvedValue({ connected: false })
    api.records.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
  })

  it('preserves saved credentials with empty fields and parses the group whitelist', async () => {
    const wrapper = mount(QQBotView)
    await flushPromises()
    expect(wrapper.get('#qq-token').element).toHaveProperty('value', '')
    expect(wrapper.get('#qq-key').element).toHaveProperty('value', '')
    await wrapper.get('#qq-groups').setValue('987654\n222222，333333')
    await wrapper.get('#qq-reward').setValue('0.05')
    await wrapper.get('#qq-reward-max').setValue('0.1')
    api.save.mockResolvedValue(await api.config())
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(expect.objectContaining({ allowed_groups: ['987654', '222222', '333333'], access_token: '', api_key: '', checkin_reward: 0.05, checkin_reward_max: 0.1 }))
  })

  it('tests the currently edited model with the dedicated key', async () => {
    const wrapper = mount(QQBotView)
    await flushPromises()
    await wrapper.get('#qq-model').setValue('new-model')
    api.test.mockResolvedValue({ reply: 'OK' })
    await wrapper.findAll('button').find(button => button.text() === 'qqBot.test')!.trigger('click')
    await flushPromises()
    expect(api.test).toHaveBeenCalledWith(expect.objectContaining({ chat_model: 'new-model', api_key: '' }))
    expect(wrapper.text()).toContain('qqBot.testSuccess')
  })
})
