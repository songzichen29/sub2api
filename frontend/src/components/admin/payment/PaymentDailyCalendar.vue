<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('payment.admin.dailyCalendar') }}
      </h3>
      <div class="text-xs text-gray-500 dark:text-gray-400">
        {{ activeDays }} {{ t('payment.admin.activeDays') }}
      </div>
    </div>

    <div v-if="!data.length" class="flex h-64 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('payment.admin.noData') }}
    </div>

    <div v-else class="space-y-5">
      <div v-for="month in months" :key="month.key">
        <div class="mb-2 flex items-center justify-between">
          <div class="text-sm font-medium text-gray-900 dark:text-white">
            {{ formatMonth(month.firstDay) }}
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            ¥{{ formatMoney(month.amount) }} · {{ month.count }} {{ t('payment.admin.orders') }}
          </div>
        </div>

        <div class="grid grid-cols-7 border-l border-t border-gray-200 dark:border-dark-600">
          <div
            v-for="weekday in weekdays"
            :key="weekday"
            class="border-b border-r border-gray-200 bg-gray-50 px-2 py-1 text-center text-[11px] font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400"
          >
            {{ weekday }}
          </div>

          <div
            v-for="day in month.days"
            :key="day.key"
            class="min-h-[74px] border-b border-r border-gray-200 p-2 dark:border-dark-600"
            :class="day.inMonth ? dayCellClass(day) : 'bg-gray-50/70 dark:bg-dark-800/40'"
            :title="day.inMonth ? dayTitle(day) : ''"
          >
            <template v-if="day.inMonth">
              <div class="flex items-center justify-between">
                <span
                  class="text-xs font-medium"
                  :class="day.isToday ? 'text-primary-600 dark:text-primary-400' : 'text-gray-700 dark:text-gray-300'"
                >
                  {{ day.date.getDate() }}
                </span>
                <span v-if="day.count > 0" class="text-[11px] text-gray-500 dark:text-gray-400">
                  {{ day.count }}
                </span>
              </div>
              <template v-if="hasActivity(day)">
                <div class="mt-2 text-xs font-medium text-gray-900 dark:text-white">
                  ¥{{ compactMoney(day.amount) }}
                </div>
                <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                  {{ day.count }} {{ t('payment.admin.orders') }}
                </div>
              </template>
              <div v-else class="mt-4 text-xs text-gray-300 dark:text-gray-600">—</div>
            </template>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 text-[11px] text-gray-500 dark:text-gray-400">
        <span>{{ t('payment.admin.lowRevenue') }}</span>
        <span class="h-3 w-5 rounded-sm border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-700"></span>
        <span class="h-3 w-5 rounded-sm bg-emerald-100 dark:bg-emerald-900/30"></span>
        <span class="h-3 w-5 rounded-sm bg-emerald-200 dark:bg-emerald-800/50"></span>
        <span class="h-3 w-5 rounded-sm bg-emerald-300 dark:bg-emerald-700/60"></span>
        <span>{{ t('payment.admin.highRevenue') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

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

const activeDays = computed(() => props.data.filter(item => item.count > 0 || item.amount > 0).length)

const months = computed<CalendarMonth[]>(() => {
  if (!props.data.length) return []

  const parsedDates = props.data
    .map(item => parseDate(item.date))
    .filter((date): date is Date => date !== null)

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
  if (ratio >= 0.75) return 'bg-emerald-300/80 dark:bg-emerald-700/60'
  if (ratio >= 0.4) return 'bg-emerald-200/80 dark:bg-emerald-800/50'
  return 'bg-emerald-100/80 dark:bg-emerald-900/30'
}

function hasActivity(day: CalendarDay): boolean {
  return day.amount > 0 || day.count > 0
}

function dayTitle(day: CalendarDay): string {
  return `${formatDateKey(day.date)} · ¥${formatMoney(day.amount)} · ${day.count} ${t('payment.admin.orders')}`
}
</script>
