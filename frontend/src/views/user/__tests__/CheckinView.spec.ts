import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import messages from '@/i18n/locales/zh/checkin'
import CheckinView from '../CheckinView.vue'
import type { UserCheckin } from '@/api/checkin'

const api = vi.hoisted(() => ({ overview: vi.fn(), claim: vi.fn(), refreshUser: vi.fn() }))
vi.mock('@/api/checkin', () => ({ checkinAPI: api }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ refreshUser: api.refreshUser }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
const fixture = (): UserCheckin => ({
  today: '2026-11-07', month: '2026-11', server_time: '2026-11-07T04:00:00Z', next_reset_at: '2026-11-07T16:00:00Z',
  enabled: true, eligible: true, checked_in: false, can_checkin: true,
  today_amount: 0, today_source: '', min_amount: 0.01, max_amount: 0.1, total_days: 6, streak_days: 6, total_amount: 0.3,
  streak_interval: 7, streak_reward: { type: 'chances', quantity: 2, amount: 0, prize_name: '', claim_instructions: '' }, today_bonus: null,
  min_recharge_total: 0, min_recharge_30d: 0, recharge_window_days: 30,
  days: Array.from({ length: 6 }, (_, index) => ({ date: `2026-11-0${index + 1}`, amount: 0.05, source: index % 2 ? 'qq' : 'website' })),
})
const wrappers: VueWrapper[] = []
function render() {
  const w = mount(CheckinView, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: messages } })], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
  wrappers.push(w); return w
}
beforeEach(() => { vi.resetAllMocks(); api.overview.mockResolvedValue(fixture()); api.refreshUser.mockResolvedValue({ balance: 1 }) })
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers() })
const claimButton = (w: VueWrapper) => w.get('[data-testid="checkin-claim"]')
const deferred = <T,>() => { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done }); return { promise, resolve } }

describe('website daily check-in', () => {
  it('uses server dates, Monday-first calendar and shared QQ/site records without binding', async () => {
    const w = render(); await flushPromises()
    expect(api.overview).toHaveBeenCalledWith('')
    expect(claimButton(w).attributes('disabled')).toBeUndefined()
    expect(w.text()).toContain('无需绑定 QQ')
    expect(w.get('[data-date="2026-11-07"]').attributes('aria-current')).toBe('date')
    expect(w.findAll('.calendar-days .is-empty')).toHaveLength(12)
    expect(w.get('[data-date="2026-11-02"]').attributes('aria-label')).toContain('已在 QQ')
    expect(w.get('[data-date="2026-11-08"]').classes()).toContain('is-future')
    expect(w.get('[data-testid="reward-amount"]').text()).toBe('0.01 – 0.10')
    expect(w.text()).toContain('再签 1 天')
    expect(w.get('details').attributes('open')).toBeUndefined()
  })
  it.each(['disabled', 'ineligible', 'qq-done'])('prevents claiming when %s and preserves calendar', async scenario => {
    api.overview.mockResolvedValue({ ...fixture(), can_checkin: false, enabled: scenario !== 'disabled', eligible: scenario !== 'ineligible', checked_in: scenario === 'qq-done', today_source: 'qq', today_amount: 0.1 })
    const w = render(); await flushPromises()
    expect(claimButton(w).attributes('disabled')).toBeDefined()
    expect(w.findAll('.is-signed')).toHaveLength(6)
    if (scenario === 'qq-done') expect(w.text()).toContain('已在 QQ 完成签到')
    await claimButton(w).trigger('click'); expect(api.claim).not.toHaveBeenCalled()
  })
  it('settles once on double click, accepts zero rewards and refreshes the account balance', async () => {
    const pending = deferred<unknown>(); api.claim.mockReturnValue(pending.promise)
    const w = render(); await flushPromises()
    await claimButton(w).trigger('click'); await claimButton(w).trigger('click')
    expect(api.claim).toHaveBeenCalledTimes(1); expect(api.claim).toHaveBeenCalledWith('2026-11-07')
    api.overview.mockResolvedValue({ ...fixture(), checked_in: true, can_checkin: false, today_source: 'website', today_amount: 0, streak_days: 7, total_days: 7 })
    pending.resolve({ date: '2026-11-07', source: 'website', amount: 0, bonus: null, already_checked_in: false }); await flushPromises()
    expect(w.text()).toContain('签到成功，连续天数已更新')
    expect(w.get('[data-testid="reward-amount"]').text()).toBe('0.00')
    expect(w.get('[data-testid="streak-count"]').text()).toBe('7')
    expect(claimButton(w).attributes('disabled')).toBeDefined(); expect(api.refreshUser).toHaveBeenCalledTimes(1)
  })
  it('retries the exact same day after uncertain network failure', async () => {
    api.claim.mockRejectedValueOnce(new Error('timeout')).mockResolvedValue({ date: '2026-11-07', source: 'website', amount: .1, already_checked_in: true, bonus: null })
    const w = render(); await flushPromises()
    await claimButton(w).trigger('click'); await flushPromises()
    expect(claimButton(w).text()).toBe('查询本次签到结果')
    api.overview.mockResolvedValue({ ...fixture(), checked_in: true, can_checkin: false, today_source: 'website', today_amount: .1 })
    await claimButton(w).trigger('click'); await flushPromises()
    expect(api.claim.mock.calls).toEqual([['2026-11-07'], ['2026-11-07']])
    expect(w.text()).toContain('奖励已到账')
  })
  it('resolves an uncertain result on refresh when QQ or another tab has checked in', async () => {
    api.claim.mockRejectedValue(new Error('timeout'))
    const w = render(); await flushPromises()
    await claimButton(w).trigger('click'); await flushPromises()
    api.overview.mockResolvedValue({ ...fixture(), checked_in: true, can_checkin: false, today_source: 'qq', today_amount: .1 })
    await w.get('[data-testid="refresh"]').trigger('click'); await flushPromises()
    expect(w.text()).toContain('已在 QQ 完成签到'); expect(w.text()).not.toContain('结果暂未确认')
    expect(api.refreshUser).toHaveBeenCalledTimes(1)
  })
  it('never silently signs tomorrow after a pending request crosses midnight', async () => {
    api.claim.mockRejectedValue(new Error('timeout'))
    const w = render(); await flushPromises()
    await claimButton(w).trigger('click'); await flushPromises()
    api.overview.mockResolvedValue({ ...fixture(), today: '2026-11-08', server_time: '2026-11-07T16:00:01Z', next_reset_at: '2026-11-08T16:00:00Z' })
    window.dispatchEvent(new Event('focus')); await flushPromises()
    expect(api.claim).toHaveBeenCalledTimes(1); expect(w.text()).toContain('签到日期已更新')
    expect(claimButton(w).text()).toBe('立即签到')
    await claimButton(w).trigger('click'); await flushPromises()
    expect(api.claim).toHaveBeenLastCalledWith('2026-11-08')
  })
  it('refreshes at the server midnight and clears timers on unmount', async () => {
    vi.useFakeTimers()
    api.overview.mockResolvedValueOnce({ ...fixture(), server_time: '2026-11-07T15:59:59Z' })
    const w = render(); await flushPromises()
    await vi.advanceTimersByTimeAsync(1600); await flushPromises()
    expect(api.overview).toHaveBeenCalledTimes(2)
    w.unmount(); await vi.advanceTimersByTimeAsync(86400000); window.dispatchEvent(new Event('focus'))
    expect(api.overview).toHaveBeenCalledTimes(2)
  })
  it('ignores stale calendar responses after rapid month navigation', async () => {
    const w = render(); await flushPromises()
    const october = deferred<UserCheckin>(), september = deferred<UserCheckin>()
    api.overview.mockReturnValueOnce(october.promise).mockReturnValueOnce(september.promise)
    await w.get('[data-testid="previous-month"]').trigger('click'); await w.get('[data-testid="previous-month"]').trigger('click')
    expect(api.overview.mock.calls.slice(-2)).toEqual([['2026-10'], ['2026-09']])
    september.resolve({ ...fixture(), month: '2026-09', days: [] }); await flushPromises()
    october.resolve({ ...fixture(), month: '2026-10', days: [] }); await flushPromises()
    expect(w.get('.calendar-toolbar h2').text()).toContain('9月')
    expect(w.find('[data-date="2026-09-01"]').exists()).toBe(true)
  })
  it('shows pending physical rewards without claiming they were credited', async () => {
    api.overview.mockResolvedValue({ ...fixture(), checked_in: true, can_checkin: false, today_source: 'website', today_amount: 0, today_bonus: { status: 'pending', reward: { type: 'physical', quantity: 1, amount: 0, prize_name: '礼品', claim_instructions: '联系站长' } } })
    const w = render(); await flushPromises()
    expect(w.get('.checkin-bonus').text()).toContain('待管理员发放')
    expect(w.get('.checkin-bonus').text()).toContain('联系站长')
    expect(w.text()).not.toContain('奖励已到账')
  })
  it('recovers from load errors without showing a false unsigned state', async () => {
    api.overview.mockRejectedValueOnce(new Error('offline'))
    const w = render(); await flushPromises()
    expect(w.text()).toContain('加载失败'); expect(w.find('[data-testid="checkin-claim"]').exists()).toBe(false)
    await w.get('[data-testid="refresh"]').trigger('click'); await flushPromises()
    expect(claimButton(w).attributes('disabled')).toBeUndefined()
  })
})
