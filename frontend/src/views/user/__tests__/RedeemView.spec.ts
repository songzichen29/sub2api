import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RedeemView from '../RedeemView.vue'

vi.mock('@/components/user/LotteryNotice.vue', () => ({ default: { template: '<div data-testid="lottery-notice" />' } }))

const { redeem, getAccountActivity, refreshUser, fetchActiveSubscriptions, showError, showWarning, showSuccess } = vi.hoisted(() => ({
  redeem: vi.fn(),
  getAccountActivity: vi.fn(),
  refreshUser: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api', () => ({
  redeemAPI: { redeem, getAccountActivity },
  authAPI: { getPublicSettings: vi.fn().mockResolvedValue({}) },
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { balance: 10, concurrency: 2 }, refreshUser }),
}))
vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ fetchActiveSubscriptions }),
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string, params?: Record<string, string>) =>
    params ? key + ' ' + Object.values(params).join(' ') : key }) }
})

async function submitCode() {
  const wrapper = mount(RedeemView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
  })
  await flushPromises()
  await wrapper.get('input#code').setValue(' REDEEM-CODE ')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  return wrapper
}

describe('RedeemView refresh after redemption', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    redeem.mockResolvedValue({ type: 'balance', value: 20, message: 'Code applied' })
    getAccountActivity.mockResolvedValue({ items: [], total: 0 })
    refreshUser.mockResolvedValue({ balance: 30, concurrency: 2 })
    fetchActiveSubscriptions.mockResolvedValue([])
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it.each(['balance', 'concurrency', 'subscription'])(
    'keeps a successful %s redemption when profile refresh fails', async (type) => {
      redeem.mockResolvedValue({ type, value: 20, message: 'Code applied' })
      refreshUser.mockRejectedValue({ status: 503, message: 'Service unavailable' })
      getAccountActivity.mockResolvedValueOnce({ items: [], total: 0 }).mockResolvedValueOnce({ total: 1, items: [{
        id: 1, code: 'REDEEM-CODE', type, value: 20, used_at: '2026-03-08T00:00:00Z',
      }] })

      const wrapper = await submitCode()

      expect(redeem).toHaveBeenCalledWith('REDEEM-CODE')
      expect(showError).not.toHaveBeenCalled()
      expect(showWarning).toHaveBeenCalledWith('redeem.userRefreshFailed')
      expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
      expect(wrapper.text()).toContain('Code applied')
      expect(wrapper.text()).not.toContain('redeem.failedToRedeem')
      expect((wrapper.get('input#code').element as HTMLInputElement).value).toBe('')
      expect((wrapper.get('input#code').element as HTMLInputElement).disabled).toBe(false)
      expect(getAccountActivity).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).toContain('REDEEM-C...')
      if (type === 'subscription') {
        expect(fetchActiveSubscriptions).toHaveBeenCalledWith(true)
      } else {
        expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
      }
      wrapper.unmount()
    }
  )

  it('pages on the server, changes size, and resets page and total after redeeming', async () => {
    getAccountActivity.mockResolvedValue({ items: [], total: 101 })
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 20)
    expect(button('pagination.previous').attributes('disabled')).toBeDefined()
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(2, 20)
    await button('pagination.previous').trigger('click')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 20)
    expect(wrapper.findAll('select option').map(o => o.text())).toEqual(['20', '50', '100'])
    await wrapper.get('select').setValue('50')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 50)
    await wrapper.get('select').setValue('100')
    await flushPromises()
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(2, 100)
    expect(button('pagination.next').attributes('disabled')).toBeDefined()
    getAccountActivity.mockResolvedValue({ items: [], total: 102 })
    await wrapper.get('input#code').setValue('NEW-CODE')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 100)
    expect(wrapper.text()).toContain('102')
    expect(button('pagination.previous').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('restores the loaded size and keeps rows and navigation usable after a size request fails', async () => {
    const item = { id: 1, code: 'OLD-ROWS', type: 'balance', value: 20, used_at: '2026-03-08T00:00:00Z' }
    getAccountActivity.mockResolvedValue({ items: [item], total: 61 })
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    await button('pagination.next').trigger('click')
    await flushPromises()
    let rejectRequest!: (error: Error) => void
    getAccountActivity.mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectRequest = reject }))
    await wrapper.get('select').setValue('50')
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 50)
    expect(button('pagination.next').attributes('disabled')).toBeDefined()
    rejectRequest(new Error('Network error'))
    await flushPromises()
    expect(wrapper.get('select').element.value).toBe('20')
    expect(wrapper.text()).toContain('OLD-ROWS')
    expect(wrapper.text()).toContain('61')
    expect(button('pagination.previous').attributes('disabled')).toBeUndefined()
    expect(button('pagination.next').attributes('disabled')).toBeUndefined()
    expect(showError).toHaveBeenCalledWith('redeem.historyLoadFailed')
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(3, 20)
    await wrapper.get('select').setValue('50')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 50)
    expect(wrapper.get('select').element.value).toBe('50')
    expect(button('pagination.previous').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it.each(['success', 'failure'])('ignores a stale history %s after a newer size request succeeds', async (outcome) => {
    let resolveOld!: (value: unknown) => void
    let rejectOld!: (error: Error) => void
    getAccountActivity.mockImplementationOnce(() => new Promise((resolve, reject) => {
      resolveOld = resolve
      rejectOld = reject
    }))
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    getAccountActivity.mockResolvedValue({ items: [{
      id: 2, code: 'NEW-ROWS', type: 'balance', value: 30, used_at: '2026-03-08T00:00:00Z',
    }], total: 61 })
    // Force overlapping requests to exercise responses arriving out of order.
    await wrapper.get('select').setValue('50')
    await flushPromises()
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    await button('pagination.next').trigger('click')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(2, 50)
    if (outcome === 'success') {
      resolveOld({ items: [], total: 0 })
    } else {
      rejectOld(new Error('Stale network error'))
    }
    await flushPromises()
    expect(wrapper.get('select').element.value).toBe('50')
    expect(wrapper.text()).toContain('NEW-ROWS')
    expect(wrapper.text()).toContain('61')
    expect(button('pagination.previous').attributes('disabled')).toBeUndefined()
    expect(button('pagination.next').attributes('disabled')).toBeDefined()
    expect(wrapper.get('select').attributes('disabled')).toBeUndefined()
    expect(showError).not.toHaveBeenCalled()
    await button('pagination.previous').trigger('click')
    await flushPromises()
    expect(getAccountActivity).toHaveBeenLastCalledWith(1, 50)
    wrapper.unmount()
  })

  it('disables both paging buttons for empty history', async () => {
    const wrapper = await submitCode()
    for (const button of wrapper.findAll('button').filter(b => b.text().startsWith('pagination.'))) {
      expect(button.attributes('disabled')).toBeDefined()
    }
    wrapper.unmount()
  })

  it('shows transfers and translates machine-written QQ reward receipts', async () => {
    getAccountActivity.mockResolvedValue({ total: 3, items: [
      { id: 1, code: '', type: 'transfer_out', value: -1, fee: '0.01000000', counterparty: 'to@example.test', used_at: '2026-11-03T06:00:00Z' },
      { id: 2, code: '', type: 'transfer_in', value: 0.99, fee: '0.01000000', counterparty: 'from@example.test', used_at: '2026-11-03T06:01:00Z' },
      { id: 1, code: 'QQ-REWARD', type: 'admin_balance', value: 2, notes: 'QQ growth reward #3', used_at: '2026-11-03T06:02:00Z' },
    ] })
    const wrapper = mount(RedeemView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('redeem.transferOut')
    expect(wrapper.text()).toContain('redeem.transferIn')
    expect(wrapper.text()).toContain('to@example.test')
    expect(wrapper.text()).toContain('from@example.test')
    expect(wrapper.text()).toContain('redeem.transferFee 0.01')
    expect(wrapper.text()).toContain('-$1.00')
    expect(wrapper.text()).toContain('+$0.99')
    expect(wrapper.text()).toContain('redeem.qqRewards.growth')
    expect(wrapper.text()).toContain('redeem.qqRewardRecord 3')
    expect(wrapper.text()).not.toContain('QQ growth reward #3')
    expect(wrapper.text()).not.toContain('...')
    wrapper.unmount()
  })

  it('finishes normally without a warning when profile refresh succeeds', async () => {
    const wrapper = await submitCode()

    expect(refreshUser).toHaveBeenCalledOnce()
    expect(showWarning).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
    expect((wrapper.get('input#code').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })

  it('preserves the existing subscription refresh warning after successful redemption', async () => {
    redeem.mockResolvedValue({ type: 'subscription', value: 20, message: 'Code applied' })
    fetchActiveSubscriptions.mockRejectedValue(new Error('Network Error'))
    const wrapper = await submitCode()

    expect(showWarning).toHaveBeenCalledWith('redeem.subscriptionRefreshFailed')
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('redeem.codeRedeemSuccess')
    expect(getAccountActivity).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps the code and reports failure when the redemption request itself fails', async () => {
    redeem.mockRejectedValue({ response: { data: { detail: 'Invalid code' } } })
    const wrapper = await submitCode()

    expect(showError).toHaveBeenCalledWith('redeem.redeemFailed')
    expect(wrapper.text()).toContain('Invalid code')
    expect(wrapper.text()).not.toContain('Code applied')
    expect((wrapper.get('input#code').element as HTMLInputElement).value).toBe(' REDEEM-CODE ')
    expect(refreshUser).not.toHaveBeenCalled()
    expect(fetchActiveSubscriptions).not.toHaveBeenCalled()
    expect(getAccountActivity).toHaveBeenCalledOnce()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showWarning).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
