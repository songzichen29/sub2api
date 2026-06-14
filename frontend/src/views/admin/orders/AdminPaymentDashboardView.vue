<template>
  <AppLayout>
    <div class="space-y-5">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <OrderStatsCards :stats="stats" />

        <div class="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_420px]">
          <div class="space-y-5">
            <DailyRevenueChart
              :data="stats.daily_series || []"
              :loading="loading"
              :days="days"
              :day-options="DAYS_OPTIONS"
              @update:days="days = $event"
              @refresh="loadDashboard"
            />
            <LatestOrdersCard :orders="latestOrders" :loading="ordersLoading" />
          </div>

          <div class="space-y-5">
            <PaymentDailyCalendar :data="stats.daily_series || []" />
            <PaymentMethodDonut :methods="stats.payment_methods || []" />
            <div class="card p-5">
              <h3 class="mb-4 text-base font-semibold text-gray-900 dark:text-white">
                {{ t('payment.admin.overviewPanel') }}
              </h3>
              <div class="grid grid-cols-2 gap-3">
                <div v-for="item in overviewItems" :key="item.key" class="rounded-xl border border-gray-200 p-3 dark:border-dark-600">
                  <div :class="['mb-3 flex h-9 w-9 items-center justify-center rounded-lg', item.iconClass]">
                    <Icon :name="item.icon" size="sm" :stroke-width="2" />
                  </div>
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                  <p class="mt-1 text-base font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
                </div>
              </div>
            </div>
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
import type { DashboardStats, PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatsCards from '@/components/admin/payment/OrderStatsCards.vue'
import DailyRevenueChart from '@/components/admin/payment/DailyRevenueChart.vue'
import PaymentDailyCalendar from '@/components/admin/payment/PaymentDailyCalendar.vue'
import PaymentMethodDonut from '@/components/admin/payment/PaymentMethodDonut.vue'
import LatestOrdersCard from '@/components/admin/payment/LatestOrdersCard.vue'

const { t } = useI18n()
const appStore = useAppStore()

const DAYS_OPTIONS = [7, 30, 90] as const
const days = ref<number>(30)
const loading = ref(false)
const ordersLoading = ref(false)
const stats = ref<DashboardStats | null>(null)
const latestOrders = ref<PaymentOrder[]>([])

const overviewItems = computed(() => {
  if (!stats.value) return []
  return [
    {
      key: 'todayOrders',
      label: t('payment.admin.todayOrders'),
      value: `${stats.value.today_count} ${t('payment.admin.orderUnit')}`,
      meta: t('payment.admin.inSelectedRange'),
      icon: 'clipboard' as const,
      iconClass: 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400',
    },
    {
      key: 'todayRevenue',
      label: t('payment.admin.todayRevenue'),
      value: `$${formatMoney(stats.value.today_amount)}`,
      meta: `${stats.value.today_count} ${t('payment.admin.orders')}`,
      icon: 'dollar' as const,
      iconClass: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400',
    },
    {
      key: 'topUsers',
      label: t('payment.admin.topUsers'),
      value: String(stats.value.top_users?.length || 0),
      meta: t('payment.admin.inSelectedRange'),
      icon: 'users' as const,
      iconClass: 'bg-purple-50 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400',
    },
    {
      key: 'avgAmount',
      label: t('payment.admin.avgAmount'),
      value: `$${formatMoney(stats.value.avg_amount)}`,
      meta: `${t('payment.admin.totalRevenue')} $${formatMoney(stats.value.total_amount)}`,
      icon: 'chart' as const,
      iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
    },
  ]
})

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

function formatMoney(value: number): string {
  return value.toFixed(2)
}

watch(days, () => loadDashboard())
onMounted(() => {
  loadDashboard()
  loadLatestOrders()
})
</script>
