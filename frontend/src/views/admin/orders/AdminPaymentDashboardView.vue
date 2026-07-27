<template>
  <AppLayout>
    <div class="min-h-[calc(100vh-7rem)] space-y-5">
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
import { ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { DailyPaymentStats, DashboardStats, PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
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
const loading = ref(false)
const ordersLoading = ref(false)
const calendarLoading = ref(false)
const stats = ref<DashboardStats | null>(null)
const latestOrders = ref<PaymentOrder[]>([])
const calendarMode = ref<CalendarMode>('month')
const calendarAnchor = ref(formatDateKey(new Date()))
const calendarSeries = ref<DailyPaymentStats[]>([])

async function loadDashboard() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getDashboard(days.value)
    stats.value = res.data
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
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

watch(days, () => loadDashboard())
watch([calendarMode, calendarAnchor], () => loadCalendarStats())

onMounted(() => {
  loadDashboard()
  loadLatestOrders()
  loadCalendarStats()
})
</script>
