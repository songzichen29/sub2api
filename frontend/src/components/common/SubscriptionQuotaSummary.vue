<template>
  <div class="mt-2 w-full space-y-2 text-left text-xs text-gray-600 dark:text-dark-300" :data-subscription-id="subscription.id">
    <div v-for="row in rows" :key="row.key" class="space-y-1">
      <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 tabular-nums">
        <span>{{ t(`userSubscriptions.${row.key}`) }}</span>
        <span>${{ row.used.toFixed(2) }} / ${{ row.limit.toFixed(2) }}</span>
      </div>
      <div class="h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600" role="progressbar" :aria-label="t(`userSubscriptions.${row.key}`)" :aria-valuemin="0" :aria-valuemax="row.limit" :aria-valuenow="row.used">
        <div class="h-full rounded-full" :class="row.used >= row.limit ? 'bg-red-500' : 'bg-emerald-500'" :style="{ width: `${Math.min(100, Math.max(0, row.used / row.limit * 100))}%` }" />
      </div>
      <div class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.totalQuotaRemaining', { amount: Math.max(0, row.limit - row.used).toFixed(2) }) }}</div>
    </div>
    <span v-if="!rows.length">{{ t('userSubscriptions.unlimited') }}</span>
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
      <span>{{ subscription.expires_at ? `${t('userSubscriptions.expires')} ${formatDateTime(subscription.expires_at)}` : t('userSubscriptions.noExpiration') }}</span>
      <span v-if="selectionStatus !== 'active'" class="text-amber-700 dark:text-amber-400">{{ t(`userSubscriptions.status.${selectionStatus}`) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserSubscription } from '@/types'
import { formatDateTime } from '@/utils/format'
import { getSubscriptionQuotaRows } from '@/utils/subscriptionQuota'

const props = defineProps<{ subscription: UserSubscription }>()
const { t } = useI18n()
const rows = computed(() => getSubscriptionQuotaRows(props.subscription))
const selectionStatus = computed(() => {
  if (props.subscription.expires_at && Date.parse(props.subscription.expires_at) <= Date.now()) return 'expired'
  if (props.subscription.starts_at && Date.parse(props.subscription.starts_at) > Date.now()) return 'not_started'
  return props.subscription.status
})
</script>
