import type { Group, UserSubscription } from '@/types'

export function buildKeySubscriptionOptions(groups: Group[], subscriptions: UserSubscription[], now = Date.now()) {
  return groups.flatMap((group) => {
    const base = { groupId: group.id, group, label: group.name }
    if (group.subscription_type !== 'subscription') {
      return [{ ...base, value: group.id as string | number, subscription: null as UserSubscription | null, disabled: false }]
    }
    return subscriptions.filter((subscription) => subscription.group_id === group.id
      && subscription.status === 'active'
      && (!subscription.starts_at || Date.parse(subscription.starts_at) <= now)
      && (!subscription.expires_at || Date.parse(subscription.expires_at) > now))
      .map((subscription) => ({
        ...base,
        value: `subscription:${subscription.id}`,
        label: `${group.name} · #${subscription.id}`,
        subscription,
        disabled: false
      }))
      .sort((a, b) => (a.subscription.expires_at ?? '').localeCompare(b.subscription.expires_at ?? '')
        || a.subscription.id - b.subscription.id)
  })
}
