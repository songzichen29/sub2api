import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SubscriptionProgressMini from '../SubscriptionProgressMini.vue'
const store = vi.hoisted(() => ({ activeSubscriptions: [] as unknown[], hasActiveSubscriptions: true, fetchActiveSubscriptions: vi.fn().mockResolvedValue(undefined) }))
vi.mock('@/stores', () => ({ useSubscriptionStore: () => store }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: { subscription: 'subscription' }, isFeatureFlagEnabled: () => true }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 8, 22, 12)) })
afterEach(() => vi.useRealTimers())
describe('subscription expiry calendar labels', () => {
  it.each([
    [new Date(2026, 8, 22, 18), 'expiresToday'],
    [new Date(2026, 8, 23, 18), 'expiresTomorrow'],
    [new Date(2026, 8, 22, 12), 'expired'],
    [new Date(2026, 8, 25, 12), 'daysRemaining'],
  ])('labels %s as %s', async (expires, label) => {
    store.activeSubscriptions = [{ id: 1, group_id: 1, expires_at: expires.toISOString(), group: { name: 'Plan' } }]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await w.get('button').trigger('click')
    expect(w.text()).toContain('subscriptionProgress.' + label)
  })
})


describe('independent subscription entries', () => {
  it('shows both same-group purchases with separate progress and identity', async () => {
    store.activeSubscriptions = [
      { id: 101, group_id: 7, expires_at: '2099-01-01T00:00:00Z', quota_limit_usd: 2000, quota_used_usd: 1500, group: { name: 'Monthly' } },
      { id: 102, group_id: 7, expires_at: '2099-02-01T00:00:00Z', quota_limit_usd: 2000, quota_used_usd: 200, group: { name: 'Monthly' } }
    ]
    const wrapper = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.text()).toContain('#101')
    expect(wrapper.text()).toContain('#102')
    expect(wrapper.text()).toContain('$1500.00/$2000.00')
    expect(wrapper.text()).toContain('$200.00/$2000.00')
    expect(wrapper.findAll('[style*="width: 75%"]')).toHaveLength(1)
    expect(wrapper.findAll('[style*="width: 10%"]')).toHaveLength(1)
  })
})
