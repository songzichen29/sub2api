import { describe, expect, it } from 'vitest'
import type { UserSubscription } from '@/types'
import {
  getSubscriptionOverdraftUsed,
  getSubscriptionUsagePriority,
  isOneTimeDailyQuota,
} from '../subscriptionQuota'

describe('subscriptionQuota', () => {
  it('uses overdraft pool as the primary usage metric when daily overdraft is enabled', () => {
    const sub = {
      allow_daily_overdraft: true,
      overdraft_limit_usd: 300,
      overdraft_used_usd: 120,
      weekly_usage_usd: 120,
      daily_usage_usd: 10,
      group: {
        daily_limit_usd: 10,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
      },
    } as UserSubscription

    expect(getSubscriptionOverdraftUsed(sub)).toBe(120)
    expect(getSubscriptionUsagePriority(sub)).toBe(40)
  })

  it('falls back to the most saturated non-overdraft window', () => {
    const sub = {
      allow_daily_overdraft: false,
      daily_usage_usd: 5,
      weekly_usage_usd: 60,
      monthly_usage_usd: 20,
      quota_limit_usd: 0,
      group: {
        daily_limit_usd: 10,
        weekly_limit_usd: 100,
        monthly_limit_usd: 50,
      },
    } as UserSubscription

    expect(getSubscriptionUsagePriority(sub)).toBe(60)
  })

  it('detects one-time daily quota cards', () => {
    expect(
      isOneTimeDailyQuota({
        starts_at: '2026-07-18T00:00:00Z',
        expires_at: '2026-07-18T23:59:59Z',
      })
    ).toBe(true)
  })
})
