import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import messages from '@/i18n/locales/zh/lottery'
import LotteryView from '../LotteryView.vue'
import LotteryNotice from '@/components/user/LotteryNotice.vue'
import type { UserLottery } from '@/api/lottery'
import { safeJoinURL } from '@/utils/lottery'

const api = vi.hoisted(() => ({ overview: vi.fn(), membership: vi.fn(), draws: vi.fn(), spin: vi.fn() }))
vi.mock('@/api/lottery', () => ({ lotteryAPI: api }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/user/profile/ProfileQQBotCard.vue', () => ({ default: { emits: ['changed'], template: '<button data-testid="bind" @click="$emit(\'changed\')">bind</button>' } }))
const fixture = (): UserLottery => ({ enabled: true, title: '幸运抽奖', period_start: '2026-11-03T09:48:32Z', recharge_step: 10, chances_per_step: 1,
  prizes: [{ name: '谢谢参与', amount: 0, probability_bps: 10000 }], guarantee_prizes: [],
  state: { user_id: 42, recharge_total: 35, earned: 3, used: 1, remaining: 2, next_recharge: 5, guarantee_every: 10, draws_until_guarantee: 9 },
  bound_qq: '', public_groups: [{ group_id: '1025362951', name: '活动群', join_url: 'https://qun.qq.com/a' }], pending_recharge: false })
const wrappers: VueWrapper[] = []
function render(component: typeof LotteryView | typeof LotteryNotice) {
  const wrapper = mount(component, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh: messages } })], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
  wrappers.push(wrapper); return wrapper
}
beforeEach(() => {
  sessionStorage.clear()
  vi.resetAllMocks()
  api.overview.mockResolvedValue(fixture())
  api.draws.mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
  api.membership.mockResolvedValue({ status: 'member', checked_at: '2026-11-06T05:00:00Z' })
})
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.useRealTimers() })

describe('user lottery journey', () => {
  it('shows earned chances and join/binding instructions without QQ binding', async () => {
    const w = render(LotteryView); await flushPromises()
    expect(w.get('[data-testid="lottery-remaining"]').text()).toBe('2')
    expect(w.text()).toContain('加入活动群并绑定 QQ')
    expect(w.text()).toContain('2026/11/03 17:48:32')
    expect(w.get('a[target="_blank"]').attributes('href')).toBe('https://qun.qq.com/a')
    expect(w.find('[data-testid="bind"]').exists()).toBe(true)
    expect(api.membership).not.toHaveBeenCalled()
  })
  it('refreshes after binding and never supplies a user ID to membership checks', async () => {
    const w = render(LotteryView); await flushPromises()
    api.overview.mockResolvedValue({ ...fixture(), bound_qq: '2904566522' })
    await w.get('[data-testid="bind"]').trigger('click'); await flushPromises()
    expect(api.membership).toHaveBeenCalledWith()
    expect(w.get('[data-testid="lottery-membership"]').text()).toContain('已在活动群')
    expect(w.text()).toContain('复制抽奖指令')
    expect(w.find('[data-testid="bind"]').exists()).toBe(false)
  })
  it.each(['unknown', 'not_member'] as const)('preserves chances for membership status %s', async status => {
    api.overview.mockResolvedValue({ ...fixture(), bound_qq: '2904566522' })
    api.membership.mockResolvedValue({ status, checked_at: '2026-11-06T05:00:00Z' })
    const w = render(LotteryView); await flushPromises()
    expect(w.get('[data-testid="lottery-remaining"]').text()).toBe('2')
    expect(w.text()).toContain(status === 'unknown' ? '暂时无法确认' : '当前不在开放的活动群')
    expect(w.text()).not.toContain('复制抽奖指令')
  })
  it('shows unavailable instead of not-in-group when the probe fails', async () => {
    api.overview.mockResolvedValue({ ...fixture(), bound_qq: '2904566522' })
    api.membership.mockRejectedValue(new Error('offline'))
    const w = render(LotteryView); await flushPromises()
    expect(w.text()).toContain('暂时无法确认')
    expect(w.text()).not.toContain('当前不在开放的活动群')
  })
  it('keeps history visible while the activity is closed', async () => {
    api.overview.mockResolvedValue({ ...fixture(), enabled: false })
    api.draws.mockResolvedValue({ page: 1, page_size: 20, total: 1, items: [{ id: 1, prize_name: '保底奖励', amount: 1, is_guaranteed: true, created_at: '2026-11-05T18:00:00Z' }] })
    const w = render(LotteryView); await flushPromises()
    expect(w.text()).toContain('抽奖活动暂未开放')
    expect(w.text()).toContain('2026/11/06 02:00:00')
    expect(w.find('a[target="_blank"]').exists()).toBe(false)
    expect(api.membership).not.toHaveBeenCalled()
  })
  it('refreshes settled chances after an in-progress recharge and stops polling', async () => {
    vi.useFakeTimers()
    api.overview.mockResolvedValueOnce({ ...fixture(), pending_recharge: true }).mockResolvedValue({ ...fixture(), state: { ...fixture().state, remaining: 3 } })
    const w = render(LotteryNotice); await flushPromises()
    expect(w.text()).toContain('同步中')
    expect(w.text()).toContain('剩余 2 次')
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(w.text()).toContain('剩余 3 次')
    expect(w.text()).not.toContain('同步中')
    await vi.advanceTimersByTimeAsync(15000); expect(api.overview).toHaveBeenCalledTimes(2)
  })
  it.each([LotteryView, LotteryNotice])('bounds settlement polling and cancels on unmount for %s', async component => {
    vi.useFakeTimers(); api.overview.mockResolvedValue({ ...fixture(), pending_recharge: true })
    const w = render(component); await flushPromises()
    await vi.advanceTimersByTimeAsync(65000); await flushPromises()
    expect(api.overview).toHaveBeenCalledTimes(13)
    expect(w.text()).toContain('请稍后刷新')
    w.unmount(); await vi.advanceTimersByTimeAsync(30000)
    expect(api.overview).toHaveBeenCalledTimes(13)
  })
  it('does not discard a slow membership result when settlement polling refreshes the same binding', async () => {
    vi.useFakeTimers()
    api.overview.mockResolvedValue({ ...fixture(), bound_qq: '2904566522', pending_recharge: true })
    let resolve!: (value: unknown) => void
    api.membership.mockReturnValue(new Promise(done => { resolve = done }))
    const w = render(LotteryView); await flushPromises()
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(api.overview).toHaveBeenCalledTimes(2)
    resolve({ status: 'member' }); await flushPromises()
    expect(w.get('[data-testid="lottery-membership"]').text()).toContain('已在活动群')
    expect(api.membership).toHaveBeenCalledTimes(1)
  })
  it('refreshes a newly bound QQ even when an older settlement poll is still in flight', async () => {
    vi.useFakeTimers()
    let resolve!: (value: unknown) => void
    api.overview.mockResolvedValueOnce({ ...fixture(), pending_recharge: true })
      .mockImplementationOnce(() => new Promise(done => { resolve = done }))
      .mockResolvedValue({ ...fixture(), bound_qq: '2904566522' })
    const w = render(LotteryView); await flushPromises()
    await vi.advanceTimersByTimeAsync(5000)
    await w.get('[data-testid="bind"]').trigger('click')
    resolve(fixture()); await flushPromises()
    expect(api.overview).toHaveBeenCalledTimes(3)
    expect(w.get('[data-testid="lottery-membership"]').text()).toContain('已在活动群')
  })
  it('paginates only the current account history and disables the last page', async () => {
    api.draws.mockResolvedValueOnce({ page: 1, page_size: 20, total: 21, items: [] })
    const w = render(LotteryView); await flushPromises()
    const next = () => w.findAll('button').find(b => b.text() === '下一页')!
    api.draws.mockResolvedValueOnce({ page: 2, page_size: 20, total: 21, items: [] })
    await next().trigger('click'); await flushPromises()
    expect(api.draws).toHaveBeenLastCalledWith(2)
    expect(next().attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('第 2 页')
  })
  it('blocks unsafe join URLs even if supplied by older settings', () => {
    expect(safeJoinURL('https://qm.qq.com/q/abc')).toBe(true)
    for (const link of ['javascript:alert(1)', 'https://qun.qq.com.evil.test/a', 'https://user@qun.qq.com/a', 'http://qun.qq.com/a']) expect(safeJoinURL(link)).toBe(false)
  })
  it('draws without QQ binding and prevents duplicate clicks', async () => {
    vi.useFakeTimers()
    const w = render(LotteryView); await flushPromises()
    let resolve!: (value: unknown) => void
    api.spin.mockImplementation(() => new Promise(done => { resolve = done }))
    const button = w.get('[data-testid="lottery-spin"]')
    expect(button.attributes('disabled')).toBeUndefined()
    await button.trigger('click'); await button.trigger('click')
    expect(api.spin).toHaveBeenCalledTimes(1)
    const request = api.spin.mock.calls[0]![0]
    expect(request).toMatch(/^[a-zA-Z0-9-]{16,64}$/)
    resolve({ request_id: request, prize: { name: '奖励', amount: 1, probability_bps: 10000 }, prize_index: 0, prizes: [{ name: '奖励', amount: 1, probability_bps: 10000 }], is_guaranteed: true, remaining: 1 })
    await flushPromises()
    expect(w.find('[data-testid="lottery-result"]').exists()).toBe(false)
    api.overview.mockResolvedValue({ ...fixture(), state: { ...fixture().state, remaining: 1 } })
    await vi.advanceTimersByTimeAsync(4100); await flushPromises()
    expect(w.get('[data-testid="lottery-result"]').text()).toContain('奖励已到账')
    expect(w.get('[data-testid="lottery-remaining"]').text()).toBe('1')
    expect(sessionStorage.getItem('lottery-pending:42')).toBeNull()
  })
  it('reuses the pending request after a timeout and a remount', async () => {
    api.spin.mockRejectedValue({ status: 0 })
    const w = render(LotteryView); await flushPromises()
    await w.get('[data-testid="lottery-spin"]').trigger('click'); await flushPromises()
    const request = api.spin.mock.calls[0]![0]
    expect(w.text()).toContain('不会重复扣除次数')
    w.unmount()
    api.overview.mockResolvedValue({ ...fixture(), state: { ...fixture().state, remaining: 0 } })
    const restored = render(LotteryView); await flushPromises()
    expect(restored.get('[data-testid="lottery-spin"]').text()).toBe('查询本次结果')
    await restored.get('[data-testid="lottery-spin"]').trigger('click'); await flushPromises()
    expect(api.spin).toHaveBeenLastCalledWith(request)
  })
  it('disables a fresh draw with no remaining chances', async () => {
    api.overview.mockResolvedValue({ ...fixture(), state: { ...fixture().state, remaining: 0 } })
    const w = render(LotteryView); await flushPromises()
    expect(w.get('[data-testid="lottery-spin"]').attributes('disabled')).toBeDefined()
    expect(w.get('.wheel-trigger').attributes('disabled')).toBeDefined()
    expect(api.spin).not.toHaveBeenCalled()
  })
  it('starts from the wheel center and locks both entry points during settlement', async () => {
    const w = render(LotteryView); await flushPromises()
    api.spin.mockReturnValue(new Promise(() => {}))
    await w.get('.wheel-trigger').trigger('click')
    expect(api.spin).toHaveBeenCalledTimes(1)
    expect(w.get('.wheel-trigger').attributes('disabled')).toBeDefined()
    expect(w.get('[data-testid="lottery-spin"]').attributes('disabled')).toBeDefined()
    await w.get('[data-testid="lottery-spin"]').trigger('click')
    expect(api.spin).toHaveBeenCalledTimes(1)
  })
})
