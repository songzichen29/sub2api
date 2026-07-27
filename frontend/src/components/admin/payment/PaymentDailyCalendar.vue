<template>
  <div class="card flex h-full min-h-[520px] flex-col p-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('payment.admin.dailyCalendar') }}
        </h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ periodLabel }}
        </p>
      </div>

      <div class="flex flex-wrap items-center justify-end gap-2">
        <div class="flex rounded-lg border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800">
          <button
            v-for="option in modeOptions"
            :key="option.value"
            type="button"
            class="px-3 py-1.5 text-xs font-medium transition-colors first:rounded-l-lg last:rounded-r-lg"
            :class="mode === option.value
              ? 'bg-primary-600 text-white'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
            @click="emit('update:mode', option.value)"
          >
            {{ option.label }}
          </button>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
            :title="previousTitle"
            :aria-label="previousTitle"
            @click="emit('previous')"
          >
            <Icon name="chevronLeft" size="xs" />
          </button>
          <button
            type="button"
            class="h-8 rounded-md border border-gray-200 px-3 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
            @click="emit('today')"
          >
            {{ currentPeriodText }}
          </button>
          <button
            type="button"
            class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
            :title="nextTitle"
            :aria-label="nextTitle"
            @click="emit('next')"
          >
            <Icon name="chevronRight" size="xs" />
          </button>
        </div>
      </div>
    </div>

    <div class="mb-3 grid grid-cols-2 gap-3">
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.totalRevenue') }}</p>
        <p
          v-for="[currency, amount] in sortedAmounts(periodAmounts)"
          :key="currency"
          class="mt-1 text-lg font-semibold text-gray-900 dark:text-white"
        >
          {{ formatMoney(currency, amount) }}
        </p>
        <p v-if="!Object.keys(periodAmounts).length" class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">-</p>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.totalOrders') }}</p>
        <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ periodCount }}</p>
      </div>
    </div>

    <div class="relative flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
      <div class="grid grid-cols-7 border-b border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800">
        <div
          v-for="weekday in weekdays"
          :key="weekday"
          class="border-r border-gray-200 px-2 py-2 text-center text-xs font-medium text-gray-500 last:border-r-0 dark:border-dark-600 dark:text-gray-400"
        >
          {{ weekday }}
        </div>
      </div>

      <div
        class="grid flex-1 auto-rows-fr grid-cols-7"
        :class="mode === 'week' ? 'min-h-[190px]' : 'min-h-[360px]'"
      >
        <div
          v-for="(day, index) in calendarDays"
          :key="day.key"
          class="border-r border-t border-gray-200 p-2 dark:border-dark-600"
          :class="[
            (index + 1) % 7 === 0 ? 'border-r-0' : '',
            day.inPeriod ? dayCellClass(day) : 'bg-gray-50/70 dark:bg-dark-800/40'
          ]"
          :title="day.inPeriod ? dayTitle(day) : ''"
        >
          <template v-if="day.inPeriod">
            <div class="flex items-center justify-between gap-1">
              <span
                class="text-xs font-medium"
                :class="day.isToday ? 'text-primary-600 dark:text-primary-400' : 'text-gray-700 dark:text-gray-300'"
              >
                {{ formatDayNumber(day.date) }}
              </span>
              <span v-if="day.count > 0" class="text-[10px] text-gray-500 dark:text-gray-400">
                {{ day.count }}
              </span>
            </div>

            <template v-if="hasActivity(day)">
              <div
                v-for="[currency, amount] in sortedAmounts(day.amounts)"
                :key="currency"
                class="mt-1 truncate text-xs font-semibold text-gray-900 first:mt-2 dark:text-white"
              >
                {{ compactMoney(currency, amount) }}
              </div>
              <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                {{ day.count }} {{ t('payment.admin.orders') }}
              </div>
            </template>
            <div v-else class="mt-2 text-xs text-gray-300 dark:text-gray-600">-</div>
          </template>
        </div>
      </div>

      <div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-white/70 dark:bg-dark-800/70">
        <LoadingSpinner size="md" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CurrencyAmounts, DailyPaymentStats } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

type CalendarMode = 'week' | 'month'

interface CalendarDay {
  key: string
  date: Date
  inPeriod: boolean
  amounts: CurrencyAmounts
  count: number
  isToday: boolean
}

const props = withDefaults(defineProps<{
  data: DailyPaymentStats[]
  loading?: boolean
  mode: CalendarMode
  anchorDate: string
}>(), {
  loading: false,
})

const emit = defineEmits<{
  (e: 'update:mode', mode: CalendarMode): void
  (e: 'previous'): void
  (e: 'next'): void
  (e: 'today'): void
}>()

const { t, locale } = useI18n()

const modeOptions = computed(() => [
  { value: 'week' as const, label: t('payment.admin.calendarWeekly') },
  { value: 'month' as const, label: t('payment.admin.calendarMonthly') },
])

const weekdays = computed(() => [
  t('payment.admin.weekdays.mon'),
  t('payment.admin.weekdays.tue'),
  t('payment.admin.weekdays.wed'),
  t('payment.admin.weekdays.thu'),
  t('payment.admin.weekdays.fri'),
  t('payment.admin.weekdays.sat'),
  t('payment.admin.weekdays.sun'),
])

const anchor = computed(() => parseDate(props.anchorDate) || new Date())

const statsByDate = computed(() => {
  const map = new Map<string, DailyPaymentStats>()
  for (const item of props.data || []) {
    map.set(item.date, item)
  }
  return map
})

const range = computed(() => props.mode === 'week' ? getWeekRange(anchor.value) : getMonthRange(anchor.value))

const calendarDays = computed(() => {
  return props.mode === 'week' ? buildWeekDays() : buildMonthDays()
})

const periodDays = computed(() => calendarDays.value.filter(day => day.inPeriod))

const periodAmounts = computed<CurrencyAmounts>(() => {
  const totals: CurrencyAmounts = {}
  for (const day of periodDays.value) {
    for (const [currency, amount] of Object.entries(day.amounts)) {
      totals[currency] = Math.round(((totals[currency] || 0) + amount) * 100) / 100
    }
  }
  return totals
})

const periodCount = computed(() => periodDays.value.reduce((sum, day) => sum + day.count, 0))

const maxAmounts = computed<CurrencyAmounts>(() => {
  const maximums: CurrencyAmounts = {}
  for (const day of periodDays.value) {
    for (const [currency, amount] of Object.entries(day.amounts)) {
      maximums[currency] = Math.max(maximums[currency] || 0, amount)
    }
  }
  return maximums
})

const periodLabel = computed(() => {
  if (props.mode === 'month') return formatMonth(range.value.start)
  const end = new Date(range.value.endExclusive)
  end.setDate(end.getDate() - 1)
  return `${formatShortDate(range.value.start)} - ${formatShortDate(end)}`
})

const previousTitle = computed(() => props.mode === 'week' ? t('payment.admin.previousWeek') : t('payment.admin.previousMonth'))
const nextTitle = computed(() => props.mode === 'week' ? t('payment.admin.nextWeek') : t('payment.admin.nextMonth'))
const currentPeriodText = computed(() => props.mode === 'week' ? t('payment.admin.currentWeek') : t('payment.admin.currentMonth'))

function buildWeekDays(): CalendarDay[] {
  const days: CalendarDay[] = []
  const todayKey = formatDateKey(new Date())

  for (let i = 0; i < 7; i++) {
    const date = new Date(range.value.start)
    date.setDate(range.value.start.getDate() + i)
    days.push(buildDay(date, true, todayKey))
  }
  return days
}

function buildMonthDays(): CalendarDay[] {
  const days: CalendarDay[] = []
  const todayKey = formatDateKey(new Date())
  const monthStart = range.value.start
  const gridStart = startOfWeek(monthStart)

  for (let i = 0; i < 42; i++) {
    const date = new Date(gridStart)
    date.setDate(gridStart.getDate() + i)
    const inPeriod = date.getFullYear() === monthStart.getFullYear() && date.getMonth() === monthStart.getMonth()
    days.push(buildDay(date, inPeriod, todayKey))
  }
  return days
}

function buildDay(date: Date, inPeriod: boolean, todayKey: string): CalendarDay {
  const key = formatDateKey(date)
  const stat = statsByDate.value.get(key)
  return {
    key,
    date,
    inPeriod,
    amounts: inPeriod ? (stat?.amount || {}) : {},
    count: inPeriod ? (stat?.count || 0) : 0,
    isToday: key === todayKey,
  }
}

function getWeekRange(date: Date) {
  const start = startOfWeek(date)
  const endExclusive = new Date(start)
  endExclusive.setDate(start.getDate() + 7)
  return { start, endExclusive }
}

function getMonthRange(date: Date) {
  const start = new Date(date.getFullYear(), date.getMonth(), 1)
  const endExclusive = new Date(date.getFullYear(), date.getMonth() + 1, 1)
  return { start, endExclusive }
}

function startOfWeek(date: Date): Date {
  const result = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  const weekday = result.getDay() || 7
  result.setDate(result.getDate() - weekday + 1)
  return result
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

function formatShortDate(date: Date): string {
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit',
    day: '2-digit',
  }).format(date)
}

function formatDayNumber(date: Date): string {
  if (props.mode === 'week') return formatShortDate(date)
  return String(date.getDate())
}

function formatMoney(currency: string, value: number): string {
  try {
    return new Intl.NumberFormat(locale.value, { style: 'currency', currency }).format(value)
  } catch {
    return `${currency} ${value.toFixed(2)}`
  }
}

function compactMoney(currency: string, value: number): string {
  try {
    return new Intl.NumberFormat(locale.value, {
      style: 'currency',
      currency,
      notation: 'compact',
      maximumFractionDigits: 2,
    }).format(value)
  } catch {
    return `${currency} ${value.toFixed(value >= 100 ? 0 : 2)}`
  }
}

function dayCellClass(day: CalendarDay): string {
  const ratios = Object.entries(day.amounts).map(([currency, amount]) => {
    return amount / (maxAmounts.value[currency] || 1)
  })
  const ratio = ratios.length ? Math.max(...ratios) : 0
  if (ratio <= 0) return 'bg-white dark:bg-dark-800'
  if (ratio >= 0.75) return 'bg-emerald-200/80 dark:bg-emerald-700/50'
  if (ratio >= 0.4) return 'bg-emerald-100/90 dark:bg-emerald-800/40'
  return 'bg-emerald-50 dark:bg-emerald-900/25'
}

function hasActivity(day: CalendarDay): boolean {
  return Object.values(day.amounts).some(amount => amount > 0) || day.count > 0
}

function dayTitle(day: CalendarDay): string {
  const amounts = sortedAmounts(day.amounts).map(([currency, amount]) => formatMoney(currency, amount)).join(' · ')
  return `${formatDateKey(day.date)} · ${amounts || '-'} · ${day.count} ${t('payment.admin.orders')}`
}

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
}
</script>
