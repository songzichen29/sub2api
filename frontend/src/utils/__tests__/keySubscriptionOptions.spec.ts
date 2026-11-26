import { describe, expect, it } from 'vitest'
import type { Group, UserSubscription } from '@/types'
import { buildKeySubscriptionOptions } from '../keySubscriptionOptions'
import { getSubscriptionQuotaRows } from '../subscriptionQuota'

describe('key subscription choices', () => {
  const group = { id: 7, name: 'Monthly', subscription_type: 'subscription', monthly_limit_usd: 2000 } as Group
  const first = { id: 101, group_id: 7, status: 'active', expires_at: '2099-01-01T00:00:00Z', monthly_usage_usd: 1500, group } as UserSubscription

  it('expands same-group instances without duplicating balance groups', () => {
    const options = buildKeySubscriptionOptions([group, { id: 8, name: 'Balance', subscription_type: 'standard' } as Group], [first, { ...first, id: 102, monthly_usage_usd: 200 }])
    expect(options.map((option) => option.value)).toEqual(['subscription:101', 'subscription:102', 8])
    expect(options[0].label).toContain('#101')
    expect(options[1].subscription?.monthly_usage_usd).toBe(200)
    expect(options[2].subscription).toBeNull()
  })

  it('omits expired records and orders active purchases by their own expiry', () => {
    const options = buildKeySubscriptionOptions([group], [
      { ...first, id: 90, expires_at: '2020-01-01T00:00:00Z' },
      first,
      { ...first, id: 102, expires_at: '2098-01-01T00:00:00Z' }
    ])
    expect(options.map((option) => option.subscription?.id)).toEqual([102, 101])
  })

  it('filters every inactive instance while retaining ordinary balance groups', () => {
    const balance = { id: 8, name: 'Balance', subscription_type: 'standard' } as Group
    const options = buildKeySubscriptionOptions([group, balance], [
      { ...first, expires_at: '2020-01-01T00:00:00Z' },
      { ...first, id: 102, status: 'revoked' },
      { ...first, id: 103, starts_at: '2098-01-01T00:00:00Z' },
      { ...first, id: 104, status: 'quota_exhausted' },
      { ...first, id: 105, status: 'suspended' },
      { ...first, id: 106, status: 'expired' }
    ], Date.parse('2026-11-26T00:00:00Z'))
    expect(options).toHaveLength(1)
    expect(options[0]).toMatchObject({ value: 8, groupId: 8, subscription: null, disabled: false })
  })

  it('includes the exact start boundary but excludes the exact expiry boundary', () => {
    const now = Date.parse('2026-11-26T00:00:00Z')
    const before = new Date(now - 1).toISOString()
    const current = new Date(now).toISOString()
    const after = new Date(now + 1).toISOString()
    const options = buildKeySubscriptionOptions([group], [
      { ...first, id: 101, starts_at: current, expires_at: after },
      { ...first, id: 102, starts_at: after },
      { ...first, id: 103, starts_at: before, expires_at: current },
      { ...first, id: 104, starts_at: before, expires_at: before },
      { ...first, id: 105, starts_at: before, expires_at: after }
    ], now)
    expect(options.map((option) => option.subscription?.id)).toEqual([101, 105])
  })

  it('preserves active subscriptions without optional start or expiry limits', () => {
    const options = buildKeySubscriptionOptions([group], [{ ...first, starts_at: null, expires_at: null }])
    expect(options.map((option) => option.subscription?.id)).toEqual([101])
  })

  it('keeps monthly and total limits distinct and uses the actual instance usage', () => {
    expect(getSubscriptionQuotaRows({ ...first, quota_limit_usd: 3000, quota_used_usd: 1800 })).toEqual([
      { key: 'totalQuota', used: 1800, limit: 3000 }, { key: 'monthly', used: 1500, limit: 2000 }
    ])
    expect(getSubscriptionQuotaRows({ ...first, allow_daily_overdraft: true, overdraft_limit_usd: 500, overdraft_used_usd: 120 })).toEqual([
      { key: 'overdraftTotal', used: 120, limit: 500 }
    ])
  })
})
