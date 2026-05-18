import type { UserSubscription } from '@/types'
import { formatRemainingDuration } from '@/utils/format'

type Translate = (key: string, params?: Record<string, unknown>) => string

export function buildRenewalNoticeText(
  subscription: UserSubscription,
  t: Translate,
  renewalDays?: number | null,
): string {
  const timeText = subscription.expires_at
    ? (formatRemainingDuration(subscription.expires_at) || t('userSubscriptions.status.expired'))
    : t('userSubscriptions.noExpiration')
  const base = `${t('payment.renewalNoticeTitle')}：${t('userSubscriptions.exactRemaining', { time: timeText })}。`
  if (renewalDays === 1) {
    return `${base}本次购买会刷新今日额度。`
  }
  return `${base}你可以选择延长时间，或重置额度重新开始。`
}

export function getRenewalModeLabel(renewalDays: number | null | undefined, t: Translate): string {
  if (renewalDays === 1) return t('payment.renewalResetNoticeTitle')
  return t('payment.renewalNoticeTitle')
}
