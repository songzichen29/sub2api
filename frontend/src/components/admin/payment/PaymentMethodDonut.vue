<template>
  <div class="card p-5">
    <h3 class="mb-4 text-base font-semibold text-gray-900 dark:text-white">
      {{ t('payment.admin.paymentDistribution') }}
    </h3>

    <div v-if="!methods?.length" class="flex h-44 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('payment.admin.noData') }}
    </div>

    <div v-else class="grid grid-cols-1 items-center gap-4 sm:grid-cols-[160px_minmax(0,1fr)] xl:grid-cols-1 2xl:grid-cols-[160px_minmax(0,1fr)]">
      <div class="relative mx-auto h-40 w-40">
        <Doughnut :data="chartData" :options="chartOptions" />
        <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.revenueShare') }}</span>
          <span class="text-lg font-semibold text-gray-900 dark:text-white">100%</span>
        </div>
      </div>

      <div class="space-y-2">
        <div v-for="item in rankedMethods" :key="item.type" class="flex items-center justify-between gap-3 text-sm">
          <div class="flex min-w-0 items-center gap-2">
            <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ backgroundColor: methodColor(item.type) }"></span>
            <span class="truncate text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + item.type, item.type) }}</span>
          </div>
          <span class="shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.percent.toFixed(1) }}%</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Doughnut } from 'vue-chartjs'

ChartJS.register(ArcElement, Tooltip, Legend)

interface PaymentMethodStat {
  type: string
  amount: number
  count: number
}

const props = defineProps<{
  methods: PaymentMethodStat[]
}>()

const { t } = useI18n()

const palette = ['#3b82f6', '#10b981', '#a855f7', '#f97316', '#f59e0b', '#64748b']

const rankedMethods = computed(() => {
  const total = props.methods.reduce((sum, item) => sum + item.amount, 0)
  return [...props.methods]
    .sort((a, b) => b.amount - a.amount)
    .map(item => ({
      ...item,
      percent: total > 0 ? (item.amount / total) * 100 : 0,
    }))
})

const chartData = computed(() => ({
  labels: rankedMethods.value.map(item => t('payment.methods.' + item.type, item.type)),
  datasets: [{
    data: rankedMethods.value.map(item => item.amount),
    backgroundColor: rankedMethods.value.map(item => methodColor(item.type)),
    borderWidth: 0,
    hoverOffset: 3,
  }]
}))

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  cutout: '64%',
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label(context: { label: string; parsed: number }) {
          return `${context.label}: $${context.parsed.toFixed(2)}`
        }
      }
    }
  }
}

function methodColor(type: string): string {
  const known: Record<string, string> = {
    alipay: '#3b82f6',
    alipay_direct: '#60a5fa',
    wxpay: '#10b981',
    wxpay_direct: '#34d399',
    stripe: '#a855f7',
    airwallex: '#f97316',
    easypay: '#f59e0b',
  }
  if (known[type]) return known[type]
  const index = Math.abs(hashString(type)) % palette.length
  return palette[index]
}

function hashString(value: string): number {
  let hash = 0
  for (let i = 0; i < value.length; i++) hash = ((hash << 5) - hash) + value.charCodeAt(i)
  return hash
}
</script>
