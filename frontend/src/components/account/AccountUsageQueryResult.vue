<template>
  <div class="account-usage-query-result space-y-2 text-xs leading-5" data-testid="usage-query-result" :aria-busy="loading">
    <div v-if="scope" class="text-gray-500 dark:text-gray-400">{{ scope }}</div>
    <div v-if="loading" class="h-3 w-24 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    <div v-else-if="error" role="alert" class="max-w-sm break-words text-amber-600 dark:text-amber-400" data-testid="usage-query-error">{{ error }}</div>
    <template v-else-if="quota">
      <UsageProgressBar
        v-if="quota.total_known !== false"
        :label="t('admin.accounts.usageWindow.used')"
        :utilization="quota.utilization * 100"
        color="indigo"
        display-mode="percentage"
        data-testid="third-party-usage-percent"
      />
      <div class="tabular-nums" data-testid="third-party-remaining">
        <span class="mr-1 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageWindow.remaining') }}</span>
        <span class="font-semibold" :class="quota.remaining <= 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-800 dark:text-gray-100'">{{ amount(quota.remaining) }}</span>
      </div>
      <div v-if="quota.updated_at" class="text-gray-500 dark:text-gray-400" data-testid="usage-query-updated-at">
        {{ t('admin.accounts.usageQuery.updatedAt') }} <time :datetime="new Date(quota.updated_at).toISOString()">{{ formatDateTime(new Date(quota.updated_at)) }}</time>
      </div>
      <details v-if="quota.total_known !== false">
        <summary class="w-fit cursor-pointer text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageQuery.details') }}</summary>
        <dl class="mt-1 grid grid-cols-[auto_1fr] gap-x-4 tabular-nums">
          <template v-if="quota.plan_name"><dt class="text-gray-500">{{ t('admin.accounts.usageQuery.plan') }}</dt><dd>{{ quota.plan_name }}</dd></template>
          <dt class="text-gray-500">{{ t('admin.accounts.usageWindow.used') }}</dt><dd>{{ amount(quota.used) }}</dd>
          <dt class="text-gray-500">{{ t('admin.accounts.usageQuery.total') }}</dt><dd>{{ amount(quota.total) }}</dd>
        </dl>
      </details>
    </template>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AccountThirdPartyQuotaInfo } from '@/types'
import { formatDateTime } from '@/utils/format'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ quota?: AccountThirdPartyQuotaInfo | null; error?: string | null; loading?: boolean; scope?: string }>()
const { t } = useI18n()
function amount(value: number) {
  const unit = props.quota!.unit
  if (unit === 'USD') return `$${value.toFixed(2)}`
  if (unit === 'CNY') return `¥${value.toFixed(2)}`
  return `${value.toFixed(2)} ${unit}`
}
</script>
