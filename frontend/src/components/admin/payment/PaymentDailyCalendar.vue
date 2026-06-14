<template>
  <div class="card p-5">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ t('payment.admin.dailyCalendar') }}
      </h3>
      <div v-if="visibleMonth" class="flex items-center gap-2">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ formatMonth(visibleMonth.firstDay) }}</span>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
          :disabled="!hasPreviousMonth"
          :title="t('payment.admin.previousMonth')"
          :aria-label="t('payment.admin.previousMonth')"
          @click="goPreviousMonth"
        >
          <Icon name="chevronLeft" size="xs" />
        </button>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
          :disabled="!hasNextMonth"
          :title="t('payment.admin.nextMonth')"
          :aria-label="t('payment.admin.nextMonth')"
          @click="goNextMonth"
        >
          <Icon name="chevronRight" size="xs" />
        </button>
      </div>
    </div>

    <div v-if="!data.length" class="flex h-64 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('payment.admin.noData') }}
    </div>

    <div v-else-if="visibleMonth" class="space-y-3">
      <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-gray-500 dark:text-gray-400">
        <span>{{ t('payment.admin.totalRevenue') }}：<span class="font-medium text-gray-700 dark:text-gray-200">${{ formatMoney(visibleMonth.amount) }}</span></span>
        <span>{{ t('payment.admin.totalOrders') }}：<span class="font-medium text-gray-700 dark:text-gray-200">{{ visibleMonth.count }}</span></span>
      </div>

      <div class="grid grid-cols-7 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
        <div
          v-for="weekday in weekdays"
          :key="weekday"
          class="border-b border-r border-gray-200 bg-gray-50 px-2 py-2 text-center text-xs font-medium text-gray-500 last:border-r-0 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400"
        >
          {{ weekday }}
        </div>

        <div
          v-for="(day, index) in visibleMonth.days"
          :key="day.key"
          class="min-h-[58px] border-r border-t border-gray-200 p-2 dark:border-dark-600"
          :class="[day.inMonth ? dayCellClass(day) : 'bg-gray-50/70 dark:bg-dark-800/40', (index + 1) % 7 === 0 ? 'border-r-0' : '']"
          :title="day.inMonth ? dayTitle(day) : ''"
        >
          <template v-if="day.inMonth">
            <div class="flex items-center justify-between gap-1">
              <span
                class="text-xs font-medium"
                :class="day.isToday ? 'text-primary-600 dark:text-primary-400' : 'text-gray-700 dark:text-gray-300'"
              >
                {{ day.date.getDate() }}
              </span>
              <span v-if="day.count > 0" class="text-[10px] text-gray-500 dark:text-gray-400">
                {{ day.count }}
              </span>
            </div>
            <template v-if="hasActivity(day)">
              <div class="mt-1.5 truncate text-xs font-semibold text-gray-900 dark:text-white">
                ${{ compactMoney(day.amount) }}
              </div>
              <div class="mt-0.5 truncate text-[10px] text-gray-500 dark:text-gray-400">
                {{ day.count }} {{ t('payment.admin.orders') }}
              </div>
            </template>
            <div v-else class="mt-2 text-xs text-gray-300 dark:text-gray-600">-</div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

interface DailyPaymentStat {
  date: string
  amount: number
  count: number
}

interface CalendarDay {
  key: string
  date: Date
  inMonth: boolean
  amount: number
  count: number
  isToday: boolean
}

interface CalendarMonth {
  key: string
  firstDay: Date
  amount: number
  count: number
  days: CalendarDay[]
}

const props = defineProps<{
  data: DailyPaymentStat[]
}>()

const { t, locale } = useI18n()
const selectedMonthKey = ref('')

const weekdays = computed(() => [
  t('payment.admin.weekdays.sun'),
  t('payment.admin.weekdays.mon'),
  t('payment.admin.weekdays.tue'),
  t('payment.admin.weekdays.wed'),
  t('payment.admin.weekdays.thu'),
  t('payment.admin.weekdays.fri'),
  t('payment.admin.weekdays.sat'),
])

const statsByDate = computed(() => {
  const map = new Map<string, DailyPaymentStat>()
  for (const item of props.data) {
    map.set(item.date, item)
  }
  return map
})

const maxAmount = computed(() => {
  return props.data.reduce((max, item) => Math.max(max, item.amount || 0), 0)
})

const months = computed<CalendarMonth[]>(() => {
  if (!props.data.length) return []

  const parsedDates = props.data
    .map(item => parseDate(item.date))
    .filter((date): date is Date => date !== null)
    .sort((a, b) => a.getTime() - b.getTime())

  if (!parsedDates.length) return []

  const first = parsedDates[0]
  const last = parsedDates[parsedDates.length - 1]
  const result: CalendarMonth[] = []
  const cursor = new Date(first.getFullYear(), first.getMonth(), 1)
  const end = new Date(last.getFullYear(), last.getMonth(), 1)

  while (cursor <= end) {
    result.push(buildMonth(cursor))
    cursor.setMonth(cursor.getMonth() + 1)
  }

  return result
})

const visibleMonth = computed(() => {
  if (!months.value.length) return null
  return months.value.find(month => month.key === selectedMonthKey.value) || months.value[months.value.length - 1]
})

const visibleMonthIndex = computed(() => {
  if (!visibleMonth.value) return -1
  return months.value.findIndex(month => month.key === visibleMonth.value?.key)
})

const hasPreviousMonth = computed(() => visibleMonthIndex.value > 0)
const hasNextMonth = computed(() => visibleMonthIndex.value >= 0 && visibleMonthIndex.value < months.value.length - 1)

watch(months, newMonths => {
  if (!newMonths.length) {
    selectedMonthKey.value = ''
    return
  }

  if (!selectedMonthKey.value || !newMonths.some(month => month.key === selectedMonthKey.value)) {
    selectedMonthKey.value = newMonths[newMonths.length - 1].key
  }
}, { immediate: true })

function buildMonth(firstDay: Date): CalendarMonth {
  const monthStart = new Date(firstDay.getFullYear(), firstDay.getMonth(), 1)
  const gridStart = new Date(monthStart)
  gridStart.setDate(monthStart.getDate() - monthStart.getDay())

  const monthKey = formatDateKey(monthStart).slice(0, 7)
  const days: CalendarDay[] = []
  let amount = 0
  let count = 0

  for (let i = 0; i < 42; i++) {
    const date = new Date(gridStart)
    date.setDate(gridStart.getDate() + i)
    const key = formatDateKey(date)
    const stat = statsByDate.value.get(key)
    const inMonth = date.getMonth() === monthStart.getMonth()
    const dayAmount = stat?.amount || 0
    const dayCount = stat?.count || 0

    if (inMonth) {
      amount += dayAmount
      count += dayCount
    }

    days.push({
      key,
      date,
      inMonth,
      amount: dayAmount,
      count: dayCount,
      isToday: key === formatDateKey(new Date()),
    })
  }

  return {
    key: monthKey,
    firstDay: monthStart,
    amount,
    count,
    days,
  }
}

function parseDate(value: string): Date | null {
  const parts = value.split('-').map(Number)
  if (parts.length !== 3 || parts.some(Number.isNaN)) return null
  return new Date(parts[0], parts[1] - 1, parts[2])
}

function formatDateKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function formatMonth(date: Date): string {
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: 'long',
  }).format(date)
}

function formatMoney(value: number): string {
  return value.toFixed(2)
}

function compactMoney(value: number): string {
  if (value >= 10000) return `${(value / 10000).toFixed(1)}w`
  if (value >= 1000) return `${(value / 1000).toFixed(1)}k`
  return value.toFixed(value >= 100 ? 0 : 2)
}

function dayCellClass(day: CalendarDay): string {
  if (day.amount <= 0) return 'bg-white dark:bg-dark-800'
  const ratio = maxAmount.value > 0 ? day.amount / maxAmount.value : 0
  if (ratio >= 0.75) return 'bg-emerald-200/80 dark:bg-emerald-700/50'
  if (ratio >= 0.4) return 'bg-emerald-100/90 dark:bg-emerald-800/40'
  return 'bg-emerald-50 dark:bg-emerald-900/25'
}

function hasActivity(day: CalendarDay): boolean {
  return day.amount > 0 || day.count > 0
}

function dayTitle(day: CalendarDay): string {
  return `${formatDateKey(day.date)} · $${formatMoney(day.amount)} · ${day.count} ${t('payment.admin.orders')}`
}

function goPreviousMonth() {
  const index = visibleMonthIndex.value
  if (index <= 0) return
  selectedMonthKey.value = months.value[index - 1].key
}

function goNextMonth() {
  const index = visibleMonthIndex.value
  if (index < 0 || index >= months.value.length - 1) return
  selectedMonthKey.value = months.value[index + 1].key
}
</script>
