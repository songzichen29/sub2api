<template>
  <AppLayout>
    <div class="min-h-[calc(100vh-7rem)] space-y-5">
      <div class="flex flex-wrap items-end justify-between gap-4 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('payment.admin.dashboardTitle') }}
          </h1>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ rangeDescription }}
          </p>
        </div>

        <div class="flex flex-wrap items-end justify-end gap-2">
          <div class="flex items-center rounded-lg border border-gray-200 bg-white p-0.5 dark:border-dark-600 dark:bg-dark-800">
            <button
              v-for="option in DAYS_OPTIONS"
              :key="option"
              type="button"
              class="px-3 py-1.5 text-xs font-medium transition-colors first:rounded-l-md last:rounded-r-md"
              :class="rangeMode === 'preset' && days === option
                ? 'bg-primary-600 text-white'
                : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
              @click="selectPreset(option)"
            >
              {{ t('payment.admin.lastDays', { days: option }) }}
            </button>
          </div>

          <label class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('payment.admin.rangeStart') }}</span>
            <input
              v-model="customStart"
              type="date"
              class="input h-8 w-[8.5rem] px-2 text-xs"
              :aria-label="t('payment.admin.rangeStart')"
            />
          </label>
          <label class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('payment.admin.rangeEnd') }}</span>
            <input
              v-model="customEnd"
              type="date"
              class="input h-8 w-[8.5rem] px-2 text-xs"
              :aria-label="t('payment.admin.rangeEnd')"
            />
          </label>
          <button
            type="button"
            class="btn btn-secondary h-8 px-3 text-xs"
            :disabled="loading"
            @click="applyCustomRange"
          >
            <Icon name="check" size="sm" />
            {{ t('payment.admin.applyRange') }}
          </button>
        </div>
      </div>

      <p v-if="rangeError" class="-mt-2 text-xs text-red-600 dark:text-red-400" role="alert">
        {{ rangeError }}
      </p>

      <div v-if="loading && !stats" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <OrderStatsCards :stats="stats" />

        <div class="grid min-h-[calc(100vh-19rem)] grid-cols-1 items-stretch gap-5 xl:grid-cols-[minmax(0,1fr)_520px] 2xl:grid-cols-[minmax(0,1fr)_600px]">
          <div class="flex min-h-0 flex-col gap-5">
            <DailyRevenueChart
              :data="stats.daily_series || []"
              :loading="loading"
              :days="days"
              :day-options="DAYS_OPTIONS"
              @update:days="days = $event"
              @refresh="loadDashboard"
            />
            <LatestOrdersCard :orders="latestOrders" :loading="ordersLoading" class="flex-1" />
          </div>

          <div class="flex min-h-0 flex-col gap-5">
            <PaymentDailyCalendar
              :data="calendarSeries"
              :loading="calendarLoading"
              :mode="calendarMode"
              :anchor-date="calendarAnchor"
              class="flex-1"
              @update:mode="setCalendarMode"
              @previous="goPreviousCalendarPeriod"
              @next="goNextCalendarPeriod"
              @today="goCurrentCalendarPeriod"
            />
            <PaymentMethodChart :methods="stats.payment_methods || []" />
            <TopUsersLeaderboard :users="stats.top_users || {}" />
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { DailyPaymentStats, DashboardStats, PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import OrderStatsCards from '@/components/admin/payment/OrderStatsCards.vue'
import DailyRevenueChart from '@/components/admin/payment/DailyRevenueChart.vue'
import PaymentDailyCalendar from '@/components/admin/payment/PaymentDailyCalendar.vue'
import PaymentMethodChart from '@/components/admin/payment/PaymentMethodChart.vue'
import TopUsersLeaderboard from '@/components/admin/payment/TopUsersLeaderboard.vue'
import LatestOrdersCard from '@/components/admin/payment/LatestOrdersCard.vue'

type CalendarMode = 'week' | 'month'

const { t } = useI18n()
const appStore = useAppStore()

const DAYS_OPTIONS = [7, 30, 90] as const
const days = ref<number>(30)
const rangeMode = ref<'preset' | 'custom'>('preset')
const loading = ref(false)
const ordersLoading = ref(false)
const calendarLoading = ref(false)
const stats = ref<DashboardStats | null>(null)
const latestOrders = ref<PaymentOrder[]>([])
const calendarMode = ref<CalendarMode>('month')
const calendarAnchor = ref(formatDateKey(new Date()))
const calendarSeries = ref<DailyPaymentStats[]>([])
const customStart = ref(formatDateKey(addDays(new Date(), -29)))
const customEnd = ref(formatDateKey(new Date()))
const rangeError = ref('')

const rangeDescription = computed(() => {
  if (rangeMode.value === 'custom') {
    return t('payment.admin.selectedRange', { start: customStart.value, end: customEnd.value })
  }
  return t('payment.admin.lastDays', { days: days.value })
})

async function loadDashboard() {
  loading.value = true
  try {
    const query = rangeMode.value === 'custom'
      ? {
          start: customStart.value,
          end: customEnd.value,
          timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        }
      : { days: days.value }
    const res = await adminPaymentAPI.getDashboard(query)
    stats.value = res.data
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

function selectPreset(value: number) {
  rangeError.value = ''
  rangeMode.value = 'preset'
  days.value = value
}

function applyCustomRange() {
  rangeError.value = ''
  if (!customStart.value || !customEnd.value) {
    rangeError.value = t('payment.admin.rangeRequired')
    return
  }
  if (customEnd.value < customStart.value) {
    rangeError.value = t('payment.admin.rangeInvalid')
    return
  }
  rangeMode.value = 'custom'
  void loadDashboard()
}

async function loadLatestOrders() {
  ordersLoading.value = true
  try {
    const res = await adminPaymentAPI.getOrders({ page: 1, page_size: 5 })
    latestOrders.value = res.data.items || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    ordersLoading.value = false
  }
}

async function loadCalendarStats() {
  const range = getCalendarRange(parseDate(calendarAnchor.value) || new Date(), calendarMode.value)
  calendarLoading.value = true
  try {
    const res = await adminPaymentAPI.getDailyStats({
      start: formatDateKey(range.start),
      end: formatDateKey(range.end),
    })
    calendarSeries.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    calendarLoading.value = false
  }
}

function setCalendarMode(mode: CalendarMode) {
  if (calendarMode.value === mode) return
  calendarMode.value = mode
}

function goPreviousCalendarPeriod() {
  const anchor = parseDate(calendarAnchor.value) || new Date()
  if (calendarMode.value === 'week') {
    anchor.setDate(anchor.getDate() - 7)
    calendarAnchor.value = formatDateKey(anchor)
  } else {
    calendarAnchor.value = formatDateKey(new Date(anchor.getFullYear(), anchor.getMonth() - 1, 1))
  }
}

function goNextCalendarPeriod() {
  const anchor = parseDate(calendarAnchor.value) || new Date()
  if (calendarMode.value === 'week') {
    anchor.setDate(anchor.getDate() + 7)
    calendarAnchor.value = formatDateKey(anchor)
  } else {
    calendarAnchor.value = formatDateKey(new Date(anchor.getFullYear(), anchor.getMonth() + 1, 1))
  }
}

function goCurrentCalendarPeriod() {
  calendarAnchor.value = formatDateKey(new Date())
}

function getCalendarRange(anchor: Date, mode: CalendarMode): { start: Date; end: Date } {
  if (mode === 'week') {
    const start = startOfWeek(anchor)
    const end = new Date(start)
    end.setDate(start.getDate() + 6)
    return { start, end }
  }

  const start = new Date(anchor.getFullYear(), anchor.getMonth(), 1)
  const end = new Date(anchor.getFullYear(), anchor.getMonth() + 1, 0)
  return { start, end }
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

function addDays(date: Date, daysToAdd: number): Date {
  const result = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  result.setDate(result.getDate() + daysToAdd)
  return result
}

watch(days, () => {
  if (rangeMode.value === 'preset') void loadDashboard()
})
watch([calendarMode, calendarAnchor], () => loadCalendarStats())

onMounted(() => {
  loadDashboard()
  loadLatestOrders()
  loadCalendarStats()
})
</script>
