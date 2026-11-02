import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ProfileQQBotCard from '../ProfileQQBotCard.vue'

const api = vi.hoisted(() => ({ binding: vi.fn(), code: vi.fn(), confirm: vi.fn(), unbind: vi.fn() }))
vi.mock('@/api/qqbot', () => ({ qqbotApi: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, string>) => params?.qq ? `${key}:${params.qq}` : key }) }))
enableAutoUnmount(afterEach)
const button = (wrapper: VueWrapper, key: string) => wrapper.findAll('button').find(item => item.text() === key)!

describe('ProfileQQBotCard', () => {
  beforeEach(() => { vi.resetAllMocks(); api.binding.mockResolvedValue({ enabled: true, bot_qq: '123456', binding: null, pending_qq: '' }) })

  it('does not offer binding while the feature is disabled', async () => {
    api.binding.mockResolvedValue({ enabled: false, binding: null })
    const wrapper = mount(ProfileQQBotCard)
    await flushPromises()
    expect(wrapper.text()).toContain('qqBot.disabled')
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(api.code).not.toHaveBeenCalled()
  })

  it('shows a one-time command and confirms the QQ shown by the server', async () => {
    const wrapper = mount(ProfileQQBotCard)
    await flushPromises()
    api.code.mockResolvedValue({ code: 'testcode', expires_at: '2026-11-03T01:00:00Z' })
    api.binding.mockResolvedValue({ enabled: true, bot_qq: '123456', binding: null, pending_qq: '777777' })
    await button(wrapper, 'qqBot.createCode').trigger('click')
    await flushPromises()
    expect(wrapper.get('code').text()).toBe('/绑定 testcode')
    expect(wrapper.text()).toContain('qqBot.pending:777777')
    api.confirm.mockResolvedValue({})
    api.binding.mockResolvedValue({ enabled: true, binding: { qq: '777777' }, pending_qq: '' })
    await button(wrapper, 'qqBot.confirm').trigger('click')
    await flushPromises()
    expect(api.confirm).toHaveBeenCalledWith('777777')
    expect(wrapper.text()).toContain('qqBot.bound:777777')
    expect(wrapper.find('code').exists()).toBe(false)
  })

  it('does not make duplicate requests while generating a code', async () => {
    let resolve!: (value: unknown) => void
    api.code.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mount(ProfileQQBotCard)
    await flushPromises()
    await button(wrapper, 'qqBot.createCode').trigger('click')
    expect(button(wrapper, 'qqBot.createCode').attributes('disabled')).toBeDefined()
    await button(wrapper, 'qqBot.refresh').trigger('click')
    expect(api.binding).toHaveBeenCalledTimes(1)
    resolve({ code: 'code', expires_at: '2026-11-03T01:00:00Z' })
    await flushPromises()
    expect(api.code).toHaveBeenCalledTimes(1)
  })

  it('shows actionable errors returned by the site API interceptor', async () => {
    const wrapper = mount(ProfileQQBotCard)
    await flushPromises()
    api.code.mockRejectedValue({ status: 409, message: 'binding changed; refresh and try again' })
    await button(wrapper, 'qqBot.createCode').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('binding changed; refresh and try again')
  })
})
