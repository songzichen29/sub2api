<template>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
    <div v-for="item in cards" :key="item.key" class="card p-5">
      <div class="flex items-center justify-between gap-4">
        <div class="flex min-w-0 items-center gap-4">
          <div :class="['flex h-12 w-12 shrink-0 items-center justify-center rounded-xl', item.iconClass]">
            <Icon :name="item.icon" size="lg" :stroke-width="2" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-1.5">
              <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
              <Icon name="infoCircle" size="xs" class="shrink-0 text-gray-400 dark:text-gray-500" />
            </div>
            <p class="mt-1 text-xl font-semibold leading-tight text-gray-900 dark:text-white">{{ item.value }}</p>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
          </div>
        </div>

        <svg class="hidden h-10 w-20 shrink-0 text-current opacity-70 sm:block" :class="item.sparkClass" viewBox="0 0 80 40" fill="none" aria-hidden="true">
          <path :d="item.area" fill="currentColor" opacity="0.12" />
          <path :d="item.line" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { CurrencyAmounts, DashboardStats } from '@/types/payment'

const props = defineProps<{
  stats: DashboardStats
}>()

const { t } = useI18n()

const cards = computed(() => [
  {
    key: 'today_amount',
    label: t('payment.admin.todayRevenue'),
    value: formatCurrencyAmounts(props.stats.today_amount),
    meta: `${props.stats.today_count} ${t('payment.admin.orders')}`,
    icon: 'creditCard' as const,
    iconClass: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400',
    sparkClass: 'text-emerald-500',
    line: 'M2 30 C12 30 12 20 22 20 C30 20 30 25 38 25 C48 25 46 14 56 14 C66 14 64 8 72 8 C76 8 76 4 78 4',
    area: 'M2 40 L2 30 C12 30 12 20 22 20 C30 20 30 25 38 25 C48 25 46 14 56 14 C66 14 64 8 72 8 C76 8 76 4 78 4 L78 40 Z',
  },
  {
    key: 'total_amount',
    label: t('payment.admin.totalRevenue'),
    value: formatCurrencyAmounts(props.stats.total_amount),
    meta: `${props.stats.total_count} ${t('payment.admin.orders')}`,
    icon: 'creditCard' as const,
    iconClass: 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400',
    sparkClass: 'text-blue-500',
    line: 'M2 26 C10 28 14 24 22 25 C30 26 30 16 38 16 C48 16 46 25 56 25 C66 25 64 8 72 8 C76 8 76 2 78 2',
    area: 'M2 40 L2 26 C10 28 14 24 22 25 C30 26 30 16 38 16 C48 16 46 25 56 25 C66 25 64 8 72 8 C76 8 76 2 78 2 L78 40 Z',
  },
  {
    key: 'today_count',
    label: t('payment.admin.todayOrders'),
    value: String(props.stats.today_count),
    meta: `${t('payment.admin.avgAmount')} ${formatCurrencyAmounts(props.stats.avg_amount)}`,
    icon: 'clipboard' as const,
    iconClass: 'bg-purple-50 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400',
    sparkClass: 'text-purple-500',
    line: 'M2 30 C12 32 12 22 22 22 C28 22 28 12 34 12 C42 12 42 22 50 22 C58 22 58 16 66 16 C72 16 72 18 78 17',
    area: 'M2 40 L2 30 C12 32 12 22 22 22 C28 22 28 12 34 12 C42 12 42 22 50 22 C58 22 58 16 66 16 C72 16 72 18 78 17 L78 40 Z',
  },
  {
    key: 'avg_amount',
    label: t('payment.admin.avgAmount'),
    value: formatCurrencyAmounts(props.stats.avg_amount),
    meta: `${t('payment.admin.totalRevenue')} ${formatCurrencyAmounts(props.stats.total_amount)}`,
    icon: 'database' as const,
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
    sparkClass: 'text-amber-500',
    line: 'M2 30 C10 30 12 10 22 10 C30 10 30 28 40 28 C50 28 48 22 58 22 C68 22 66 10 76 10',
    area: 'M2 40 L2 30 C10 30 12 10 22 10 C30 10 30 28 40 28 C50 28 48 22 58 22 C68 22 66 10 76 10 L76 40 Z',
  },
])

function formatCurrencyAmounts(amounts: CurrencyAmounts): string {
  const values = Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
  if (!values.length) return '-'
  return values.map(([currency, amount]) => formatMoney(currency, amount)).join(' · ')
}

function formatMoney(currency: string, amount: number): string {
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
  } catch {
    return `${currency} ${amount.toFixed(2)}`
  }
}
</script>
