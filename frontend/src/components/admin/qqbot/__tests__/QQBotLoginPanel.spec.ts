import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import QQBotLoginPanel from '../QQBotLoginPanel.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const api = vi.hoisted(() => ({ login: vi.fn(), refreshLoginQR: vi.fn(), restartLogin: vi.fn() }))
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
    await w.findAll('button').find(b => b.text() === zh.qqbot.login.refreshQR)!.trigger('click'); await flushPromises()
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
    { ...base, phase: 'reconnecting' },
    { ...base, phase: 'qrcode_scanned' }
  ])('hides QR while online or confirming', async status => {
    api.login.mockResolvedValue(status); const w = show(); await flushPromises()
    expect(w.find('img').exists()).toBe(false); expect(qr.toDataURL).not.toHaveBeenCalled()
    if (!status.online) {
      expect(w.findAll('button').find(b => b.text() === zh.qqbot.login.refreshQR)!.attributes('disabled')).toBeDefined()
      expect(w.get('[data-testid="qq-restart-login"]').attributes('disabled')).toBeDefined()
    }
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
  it('shows QR recovery after going offline and the new QR on the next poll', async () => {
    api.login.mockResolvedValueOnce({ ...base, online: true, connected: true, auto_recovery: true })
    const w = show(); await flushPromises()
    expect(w.get('[data-testid="login-auto-recovery"]').text()).toContain('自动恢复')
    expect(w.find('img').exists()).toBe(false)
    api.login.mockResolvedValue({ ...base, auto_recovery: true, qr_code: 'https://qq.com/recovered' })
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(qr.toDataURL).toHaveBeenLastCalledWith('https://qq.com/recovered', expect.any(Object))
    expect(w.find('img').exists()).toBe(true)
    expect(api.refreshLoginQR).not.toHaveBeenCalled()
  })
  it('does not request a QR when QQ is online but OneBot is disconnected', async () => {
    api.login.mockResolvedValue({ ...base, online: true, connected: false, auto_recovery: true })
    const w = show(); await flushPromises()
    expect(w.text()).toContain(zh.qqbot.login.connectionHelp)
    expect(w.find('img').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(15000); await flushPromises()
    expect(api.refreshLoginQR).not.toHaveBeenCalled()
  })
  it.each(['degraded', 'stale', 'offline_reported'])('does not claim normal login when session is %s', async state => {
    api.login.mockResolvedValue({ ...base, online: true, connected: true, session_health: { state, probe: 'failed', failures: 3, alert: true, last_message_at: '2026-11-03T11:13:40Z' } })
    const w = show(); await flushPromises()
    expect(w.text()).toContain(zh.qqbot.login.sessionProblem)
    expect(w.find('[data-testid="qq-session-alert"]').exists()).toBe(true)
    expect(w.text()).not.toContain(zh.qqbot.login.online)
    expect(w.find('img').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(api.refreshLoginQR).not.toHaveBeenCalled()
  })
  it('never presents an old API without health evidence as verified reception', async () => {
    api.login.mockResolvedValue({ ...base, online: true, connected: true })
    const w = show(); await flushPromises()
    expect(w.get('[data-testid="qq-session-health"]').text()).toBe(zh.qqbot.login.session.unknown)
    expect(w.text()).toContain(zh.qqbot.login.receptionHelp)
  })
  it('requires confirmation, then follows a new QR without repeating the restart', async () => {
    api.login.mockResolvedValue({ ...base, online: true, connected: true })
    const w = show(); await flushPromises()
    await w.get('[data-testid="qq-restart-login"]').trigger('click')
    expect(w.getComponent(ConfirmDialog).props('message')).toContain('1849138657')
    expect(api.restartLogin).not.toHaveBeenCalled()
    w.getComponent(ConfirmDialog).vm.$emit('cancel'); await flushPromises()
    expect(api.restartLogin).not.toHaveBeenCalled()
    await w.get('[data-testid="qq-restart-login"]').trigger('click')
    api.restartLogin.mockResolvedValue({ accepted: true, restart_available_at: new Date(Date.now() + 60000).toISOString() })
    w.getComponent(ConfirmDialog).vm.$emit('confirm'); await flushPromises()
    expect(api.restartLogin).toHaveBeenCalledOnce()
    expect(api.restartLogin).toHaveBeenCalledWith('1849138657', expect.any(AbortSignal))
    expect(w.text()).toContain(zh.qqbot.login.restartAccepted)
    api.login.mockRejectedValueOnce(new Error('worker restarting'))
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    api.login.mockResolvedValue({ ...base, qr_code: 'https://qq.com/restarted' })
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(qr.toDataURL).toHaveBeenLastCalledWith('https://qq.com/restarted', expect.any(Object))
    expect(w.get('[data-testid="qq-restart-login"]').attributes('disabled')).toBeDefined()
    expect(api.restartLogin).toHaveBeenCalledOnce()
    expect(api.refreshLoginQR).not.toHaveBeenCalled()
  })
  it('does not retry an uncertain restart or claim success', async () => {
    const w = show(); await flushPromises()
    api.restartLogin.mockRejectedValue(new Error('timeout private-token'))
    await w.get('[data-testid="qq-restart-login"]').trigger('click')
    w.getComponent(ConfirmDialog).vm.$emit('confirm'); await flushPromises()
    expect(w.text()).toContain(zh.qqbot.login.restartFailed)
    expect(w.text()).not.toContain('private-token')
    expect(w.find('[data-testid="qq-restart-notice"]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(15000); await flushPromises()
    expect(api.restartLogin).toHaveBeenCalledOnce()
    expect(w.get('[data-testid="qq-restart-login"]').attributes('disabled')).toBeDefined()
  })
})
