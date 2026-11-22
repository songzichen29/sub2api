import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser } from '@/types'
import UserBalanceHistoryModal from '../UserBalanceHistoryModal.vue'

const mocks = vi.hoisted(() => ({ getUserBalanceHistory: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: mocks } }))
vi.mock('@/utils/format', () => ({ formatDateTime: () => 'date' }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params?: Record<string, string>) =>
    params ? key + ' ' + Object.values(params).join(' ') : key }),
}))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks(); vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => vi.restoreAllMocks())
function result(id: number) {
  return { items: [{ id, type: 'admin_balance', value: id, notes: `History ${id}` }], total: 1, total_recharged: id }
}
function deferred() {
  let resolve!: (value: ReturnType<typeof result>) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<ReturnType<typeof result>>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
async function openDialog() {
  const wrapper = mount(UserBalanceHistoryModal, {
    props: { show: false, user: { id: 1, email: 'one@example.com', balance: 1 } as AdminUser },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
        Icon: true,
        Select: true,
        UserSubscriptionsSection: true,
      },
    },
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('UserBalanceHistoryModal request ordering', () => {
  it.each([
    ['QQ growth reward #76', 'growth'],
    ['QQ checkin reward #77', 'checkin'],
    ['QQ invite reward #78', 'invite'],
    ['QQ lottery reward #79', 'lottery'],
    ['QQ streak reward #80', 'streak'],
    ['QQ ranking reward #81', 'ranking'],
    ['QQ redemption reward #82', 'redemption'],
    ['Website checkin reward #83', 'website_checkin'],
  ])('localizes the system reward title and note for %s', async (notes, kind) => {
    mocks.getUserBalanceHistory.mockResolvedValue({
      items: [{ id: 1, code: 'QQ-REWARD', type: 'admin_balance', value: 0.1, notes }],
      total: 1,
      total_recharged: 0,
    })
    const wrapper = await openDialog()
    await flushPromises()
    const number = notes.match(/#(\d+)/)![1]
    expect(wrapper.text()).toContain(`redeem.qqRewards.${kind}`)
    expect(wrapper.get(`[title="redeem.qqRewardRecord ${number}"]`).text()).toBe(`redeem.qqRewardRecord ${number}`)
    expect(wrapper.text()).not.toContain(notes)
    expect(wrapper.text()).not.toContain('redeem.balanceAddedAdmin')
    expect(wrapper.text()).not.toContain('redeem.adminAdjustment')
    expect(wrapper.text()).toContain('+$0.10')
  })

  it('localizes campaign notes while retaining administrator-written adjustment notes', async () => {
    mocks.getUserBalanceHistory.mockResolvedValue({
      items: [
        { id: 1, code: 'CODE', type: 'balance', value: 1, notes: 'QQ campaign 1c712861ba2a48048bde15d9281fd793' },
        { id: 2, code: 'MANUAL', type: 'admin_balance', value: 2, notes: 'Manual credit: QQ growth reward #76' },
      ],
      total: 2,
      total_recharged: 1,
    })
    const wrapper = await openDialog()
    await flushPromises()
    expect(wrapper.text()).toContain('redeem.qqCampaignRecord')
    expect(wrapper.text()).not.toContain('QQ campaign')
    expect(wrapper.get('[title="Manual credit: QQ growth reward #76"]').text()).toBe('Manual credit: QQ growth reward #76')
    expect(wrapper.text()).toContain('redeem.balanceAddedAdmin')
    expect(wrapper.text()).toContain('redeem.adminAdjustment')
  })

  it('keeps the new user history when an old response finishes later', async () => {
    const old = deferred()
    mocks.getUserBalanceHistory.mockReturnValueOnce(old.promise).mockResolvedValueOnce(result(20))
    const wrapper = await openDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, user: { id: 2, email: 'two@example.com', balance: 2 } as AdminUser })
    await flushPromises()
    old.resolve(result(10))
    await flushPromises()
    expect(wrapper.text()).toContain('History 20')
    expect(wrapper.text()).not.toContain('History 10')
    expect(wrapper.text()).toContain('$20.00')
  })

  it('does not end the current filter loading state when an old request finishes', async () => {
    const old = deferred()
    const current = deferred()
    mocks.getUserBalanceHistory.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await openDialog()
    wrapper.findComponent({ name: 'Select' }).vm.$emit('change', 'balance')
    await flushPromises()
    old.resolve(result(10))
    await flushPromises()
    expect(wrapper.find('svg.animate-spin').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('History 10')
    current.resolve(result(20))
    await flushPromises()
    expect(wrapper.find('svg.animate-spin').exists()).toBe(false)
    expect(wrapper.text()).toContain('History 20')
  })

  it.each(['close', 'unmount'])('ignores failures after %s', async (action) => {
    const pending = deferred()
    mocks.getUserBalanceHistory.mockReturnValueOnce(pending.promise)
    const wrapper = await openDialog()
    if (action === 'close') await wrapper.setProps({ show: false })
    else wrapper.unmount()
    pending.reject(new Error('Stale error'))
    await flushPromises()
    expect(console.error).not.toHaveBeenCalled()
  })

  it('still reports current failures and ends loading', async () => {
    const error = new Error('Current error')
    mocks.getUserBalanceHistory.mockRejectedValueOnce(error)
    const wrapper = await openDialog()
    await flushPromises()
    expect(console.error).toHaveBeenCalledWith('Failed to load balance history:', error)
    expect(wrapper.find('svg.animate-spin').exists()).toBe(false)
  })
})
