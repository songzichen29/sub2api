import type { UserSubscription } from '@/types'

const ONE_DAY_MS = 24 * 60 * 60 * 1000

export interface RemainingDurationParts {
  days: number
  hours: number
  minutes: number
}

export function isOneTimeDailyQuota(
  subscription: Pick<UserSubscription, 'starts_at' | 'expires_at'>
): boolean {
  if (!subscription.starts_at || !subscription.expires_at) return false

  const startsAt = new Date(subscription.starts_at).getTime()
  const expiresAt = new Date(subscription.expires_at).getTime()

  if (!Number.isFinite(startsAt) || !Number.isFinite(expiresAt)) return false

  return expiresAt <= startsAt + ONE_DAY_MS
}

export function getSubscriptionOverdraftLimit(
  subscription: Pick<UserSubscription, 'allow_daily_overdraft' | 'overdraft_limit_usd'>
): number | null {
  return subscription.allow_daily_overdraft
    && typeof subscription.overdraft_limit_usd === 'number'
    && subscription.overdraft_limit_usd > 0
    ? subscription.overdraft_limit_usd
    : null
}

export function getSubscriptionOverdraftUsed(
  subscription: Pick<UserSubscription, 'overdraft_used_usd' | 'weekly_usage_usd'>
): number {
  return subscription.overdraft_used_usd ?? subscription.weekly_usage_usd ?? 0
}

export function getSubscriptionUsagePriority(sub: UserSubscription): number {
  const overdraftLimit = getSubscriptionOverdraftLimit(sub)
  if (overdraftLimit !== null) {
    const percentages = [
      overdraftLimit > 0
        ? (getSubscriptionOverdraftUsed(sub) / overdraftLimit) * 100
        : 0,
    ]
    if (sub.quota_limit_usd != null && sub.quota_limit_usd > 0) {
      percentages.push(((sub.quota_used_usd ?? 0) / sub.quota_limit_usd) * 100)
    }
    return Math.max(...percentages)
  }

  const percentages: number[] = []
  if (sub.quota_limit_usd != null && sub.quota_limit_usd > 0) {
    percentages.push(((sub.quota_used_usd ?? 0) / sub.quota_limit_usd) * 100)
  }
  if (sub.group?.daily_limit_usd != null && sub.group.daily_limit_usd > 0) {
    percentages.push(((sub.daily_usage_usd || 0) / sub.group.daily_limit_usd) * 100)
  }
  if (sub.group?.weekly_limit_usd != null && sub.group.weekly_limit_usd > 0) {
    percentages.push(((sub.weekly_usage_usd || 0) / sub.group.weekly_limit_usd) * 100)
  }
  if (sub.group?.monthly_limit_usd != null && sub.group.monthly_limit_usd > 0) {
    percentages.push(((sub.monthly_usage_usd || 0) / sub.group.monthly_limit_usd) * 100)
  }
  return percentages.length > 0 ? Math.max(...percentages) : 0
}

export function getRemainingDurationParts(
  targetAt: Date | string,
  now: Date = new Date()
): RemainingDurationParts | null {
  const targetTime = targetAt instanceof Date ? targetAt.getTime() : new Date(targetAt).getTime()
  const nowTime = now.getTime()

  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null

  const diffMs = targetTime - nowTime
  if (diffMs <= 0) return null

  const totalMinutes = Math.floor(diffMs / (1000 * 60))
  const days = Math.floor(totalMinutes / (24 * 60))
  const hours = Math.floor((totalMinutes % (24 * 60)) / 60)
  const minutes = totalMinutes % 60

  return { days, hours, minutes }
}
