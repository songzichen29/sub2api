import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import QQBotLoginPanel from '../QQBotLoginPanel.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

const api = vi.hoisted(() => ({ login: vi.fn(), refreshLoginQR: vi.fn() }))
const qr = vi.hoisted(() => ({ toDataURL: vi.fn() }))
vi.mock('@/api/qqcommunity', () => ({ qqbotAPI: api }))
vi.mock('qrcode', () => ({ default: qr }))
const base = { configured: true, online: false, connected: false, offline: true, phase: '', scan_accepted: false, expected_qq: '1849138657', logged_in_qq: '', account_mismatch: false, login_failed: false, qr_code: 'https://qq.com/qr' }
let wrapper: VueWrapper | undefined
function show() { wrapper = mount(QQBotLoginPanel, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh, en } })] } }); return wrapper }

describe('NapCat login panel', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.resetAllMocks(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); api.login.mockResolvedValue(base); qr.toDataURL.mockResolvedValue('data:image/png;base64,test') })
  afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.restoreAllMocks(); vi.useRealTimers() })
  it('shows an offline QR, refreshes it and stops polling on unmount', async () => {
    const w = show(); await flushPromises()
    expect(w.get('img').attributes('src')).toBe('data:image/png;base64,test')
    api.refreshLoginQR.mockResolvedValue({ ...base, qr_code: 'https://qq.com/new' })
    await w.findAll('button')[1]!.trigger('click'); await flushPromises()
    expect(api.refreshLoginQR).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(5000); await flushPromises(); expect(api.login).toHaveBeenCalledTimes(2)
    const signal = api.login.mock.calls[0]![0] as AbortSignal
    w.unmount(); wrapper = undefined
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(15000); expect(api.login).toHaveBeenCalledTimes(2)
  })
  it.each([
    { ...base, online: true, connected: true },
    { ...base, scan_accepted: true },
    { ...base, phase: 'initializing' },
    { ...base, phase: 'reconnecting' }
  ])('hides QR while online or confirming', async status => {
    api.login.mockResolvedValue(status); const w = show(); await flushPromises()
    expect(w.find('img').exists()).toBe(false); expect(qr.toDataURL).not.toHaveBeenCalled()
    if (!status.online) expect(w.findAll('button')[1]!.attributes('disabled')).toBeDefined()
  })
  it('clears stale login state on failure and retries', async () => {
    const w = show(); await flushPromises(); api.login.mockRejectedValueOnce(new Error('private failure'))
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(w.find('img').exists()).toBe(false); expect(w.get('[role="alert"]').text()).toBe(zh.qqbot.login.loadFailed)
    expect(w.text()).not.toContain('private failure')
    await vi.advanceTimersByTimeAsync(5000); await flushPromises(); expect(w.find('img').exists()).toBe(true)
  })
  it('stops polling when login integration is not configured', async () => {
    api.login.mockResolvedValue({ ...base, configured: false }); const w = show(); await flushPromises()
    expect(w.text()).toContain(zh.qqbot.login.unconfigured)
    await vi.advanceTimersByTimeAsync(15000); expect(api.login).toHaveBeenCalledOnce()
  })
})
