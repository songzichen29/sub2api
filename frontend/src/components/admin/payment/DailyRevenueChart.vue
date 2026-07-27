<template>
  <div class="card p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ t('payment.admin.revenueTrend') }}
      </h3>
      <div class="flex items-center gap-2">
        <div class="flex rounded-lg border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800">
          <button
            v-for="option in dayOptions"
            :key="option"
            type="button"
            class="px-3 py-1.5 text-xs font-medium transition-colors first:rounded-l-lg last:rounded-r-lg"
            :class="days === option
              ? 'bg-primary-600 text-white'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
            @click="$emit('update:days', option)"
          >
            {{ option }}{{ t('payment.admin.daySuffix') }}
          </button>
        </div>
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="$emit('refresh')"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>
    </div>

    <div class="h-[340px]">
      <div v-if="loading" class="flex h-full items-center justify-center">
        <LoadingSpinner size="md" />
      </div>
      <Line v-else-if="chartData" :data="chartData" :options="chartOptions" />
      <div
        v-else
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('payment.admin.noData') }}
      </div>
    </div>

    <div v-if="data.length" class="mt-5 grid grid-cols-2 gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600 lg:grid-cols-4">
      <div v-for="item in summaryItems" :key="item.key" class="flex items-center gap-3">
        <div :class="['flex h-9 w-9 shrink-0 items-center justify-center rounded-lg', item.iconClass]">
          <Icon :name="item.icon" size="sm" :stroke-width="2" />
        </div>
        <div class="min-w-0">
          <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
          <p class="mt-0.5 text-sm font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler,
  type TooltipItem
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import type { DailyPaymentStats } from '@/types/payment'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const props = withDefaults(defineProps<{
  data: DailyPaymentStats[]
  loading?: boolean
  days: number
  dayOptions: readonly number[]
}>(), {
  loading: false,
})

defineEmits<{
  (e: 'update:days', days: number): void
  (e: 'refresh'): void
}>()

const { t } = useI18n()

const colors = [
  ['rgb(59, 130, 246)', 'rgba(59, 130, 246, 0.1)'],
  ['rgb(168, 85, 247)', 'rgba(168, 85, 247, 0.1)'],
  ['rgb(245, 158, 11)', 'rgba(245, 158, 11, 0.1)'],
  ['rgb(239, 68, 68)', 'rgba(239, 68, 68, 0.1)'],
]

const currencies = computed(() => {
  return [...new Set(props.data.flatMap(day => Object.keys(day.amount)))].sort()
})

const chartData = computed(() => {
  if (!props.data || props.data.length === 0) return null
  return {
    labels: props.data.map(d => formatChartDate(d.date)),
    datasets: [
      ...currencies.value.map((currency, index) => {
        const [borderColor, backgroundColor] = colors[index % colors.length]
        return {
          label: `${currency} ${t('payment.admin.revenue')}`,
          data: props.data.map(day => day.amount[currency] || 0),
          borderColor,
          backgroundColor,
          fill: true,
          tension: 0.3,
          pointRadius: 3,
          pointHoverRadius: 5,
        }
      }),
      {
        label: t('payment.admin.orderCount'),
        data: props.data.map(d => d.count),
        borderColor: 'rgb(16, 185, 129)',
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        fill: false,
        tension: 0.35,
        pointRadius: 2.5,
        pointHoverRadius: 5,
        yAxisID: 'y1',
      }
    ]
  }
})

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  scales: {
    x: {
      grid: { display: false },
      ticks: { color: '#64748b', maxRotation: 0, autoSkip: true, maxTicksLimit: 10 },
    },
    y: {
      type: 'linear' as const,
      display: true,
      position: 'left' as const,
      title: { display: true, text: t('payment.admin.revenueWithCurrency'), color: '#64748b' },
      grid: { color: 'rgba(148, 163, 184, 0.18)' },
      ticks: { color: '#64748b' },
    },
    y1: {
      type: 'linear' as const,
      display: true,
      position: 'right' as const,
      title: { display: true, text: t('payment.admin.orderCountWithUnit'), color: '#64748b' },
      grid: { drawOnChartArea: false },
      ticks: { color: '#64748b', precision: 0 },
    }
  },
  plugins: {
    legend: {
      position: 'top' as const,
      align: 'center' as const,
      labels: { boxWidth: 28, boxHeight: 3, usePointStyle: false },
    },
    tooltip: {
      callbacks: {
        label(context: TooltipItem<'line'>) {
          const value = typeof context.parsed.y === 'number' ? context.parsed.y : 0
          if (context.dataset.yAxisID === 'y1') return `${context.dataset.label}: ${value}`
          const currency = currencies.value[context.datasetIndex]
          return `${context.dataset.label}: ${formatMoney(currency, value)}`
        }
      }
    }
  }
}))

const maxRevenueByCurrency = computed(() => {
  return currencies.value.map(currency => {
    const day = props.data.reduce<DailyPaymentStats | null>((best, item) => {
      if (!best || (item.amount[currency] || 0) > (best.amount[currency] || 0)) return item
      return best
    }, null)
    return { currency, amount: day?.amount[currency] || 0, date: day?.date || '-' }
  })
})

const maxOrdersDay = computed(() => {
  return props.data.reduce<DailyPaymentStats | null>((best, item) => {
    if (!best || item.count > best.count) return item
    return best
  }, null)
})

const averageRevenueByCurrency = computed(() => {
  if (!props.data.length) return []
  return currencies.value.map(currency => ({
    currency,
    amount: props.data.reduce((sum, item) => sum + (item.amount[currency] || 0), 0) / props.data.length,
  }))
})

const averageOrders = computed(() => {
  if (!props.data.length) return 0
  return props.data.reduce((sum, item) => sum + item.count, 0) / props.data.length
})

const summaryItems = computed(() => [
  {
    key: 'maxRevenue',
    label: t('payment.admin.maxDailyRevenue'),
    value: formatAmountList(maxRevenueByCurrency.value),
    meta: maxRevenueByCurrency.value.map(item => `${item.currency}: ${formatDate(item.date)}`).join(' · '),
    icon: 'trendingUp' as const,
    iconClass: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400',
  },
  {
    key: 'maxOrders',
    label: t('payment.admin.maxDailyOrders'),
    value: `${maxOrdersDay.value?.count || 0} ${t('payment.admin.orderUnit')}`,
    meta: formatDate(maxOrdersDay.value?.date),
    icon: 'clipboard' as const,
    iconClass: 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400',
  },
  {
    key: 'avgRevenue',
    label: t('payment.admin.avgDailyRevenue'),
    value: formatAmountList(averageRevenueByCurrency.value),
    meta: t('payment.admin.inSelectedRange'),
    icon: 'chart' as const,
    iconClass: 'bg-purple-50 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400',
  },
  {
    key: 'avgOrders',
    label: t('payment.admin.avgDailyOrders'),
    value: `${averageOrders.value.toFixed(1)} ${t('payment.admin.orderUnit')}`,
    meta: t('payment.admin.inSelectedRange'),
    icon: 'document' as const,
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
  },
])

function formatMoney(currency: string | undefined, value: number): string {
  if (!currency) return value.toFixed(2)
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(value)
}

function formatAmountList(items: { currency: string; amount: number }[]): string {
  if (!items.length) return '-'
  return items.map(item => formatMoney(item.currency, item.amount)).join(' · ')
}

function formatChartDate(value: string): string {
  const parts = value.split('-')
  if (parts.length !== 3) return value
  return `${parts[1]}-${parts[2]}`
}

function formatDate(value?: string): string {
  return value || '-'
}
</script>
