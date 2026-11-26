import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import SubscriptionsView from '../SubscriptionsView.vue'

const { getMySubscriptions, push } = vi.hoisted(() => ({ getMySubscriptions: vi.fn(), push: vi.fn() }))
vi.mock('@/api/subscriptions', () => ({ default: { getMySubscriptions } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
enableAutoUnmount(afterEach)

describe('independent subscription cards', () => {
  it('does not promise quota resets for expired, revoked or suspended purchases', async () => {
    getMySubscriptions.mockResolvedValue(['expired', 'revoked', 'suspended'].map((status, index) => ({
      id: index + 1, group_id: 7, status, expires_at: status === 'expired' ? '2020-01-01T00:00:00Z' : '2099-01-01T00:00:00Z',
      monthly_usage_usd: 100, monthly_window_start: '2020-01-01T00:00:00Z',
      group: { name: 'Monthly', platform: 'openai', monthly_limit_usd: 2000 }
    })))
    const wrapper = mount(SubscriptionsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()
    expect(wrapper.text()).not.toContain('userSubscriptions.resetIn')
  })

  it('keeps same-group purchases separate and buys another without a renewal confirmation', async () => {
    const first = { id: 101, group_id: 7, status: 'active', expires_at: '2099-01-01T00:00:00Z', monthly_usage_usd: 1500, group: { name: 'Monthly', platform: 'openai', monthly_limit_usd: 2000 } }
    getMySubscriptions.mockResolvedValue([first, { ...first, id: 102, monthly_usage_usd: 200 }])
    const wrapper = mount(SubscriptionsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('#101')
    expect(wrapper.text()).toContain('#102')
    expect(wrapper.text()).toContain('$1500.00 / $2000.00')
    expect(wrapper.text()).toContain('$200.00 / $2000.00')
    const purchaseButtons = wrapper.findAll('button').filter((button) => button.text() === 'payment.buyAnother')
    expect(purchaseButtons).toHaveLength(2)
    await purchaseButtons[1].trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/purchase', query: { tab: 'subscription', group: '7' } })
  })
})
