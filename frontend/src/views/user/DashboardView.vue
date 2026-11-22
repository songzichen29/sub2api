<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading && !stats" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <div v-if="loadErrors.stats" role="alert" class="card flex items-center justify-between gap-4 p-5">
        <p class="text-sm text-red-700 dark:text-red-400">{{ t('dashboard.statsLoadFailed') }}</p>
        <button class="btn btn-secondary" :disabled="loading" @click="loadStats">{{ t('common.retry') }}</button>
      </div>
      <template v-if="stats">
        <div v-if="loadErrors.quotas" role="alert" class="flex items-center justify-between gap-4 text-sm text-red-700 dark:text-red-400">
          <p>{{ t('dashboard.quotasLoadFailed') }}</p>
          <button class="btn btn-secondary" @click="loadPlatformQuotas">{{ t('common.retry') }}</button>
        </div>
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <div v-if="loadErrors.charts" role="alert" class="card flex items-center justify-between gap-4 p-5">
          <p class="text-sm text-red-700 dark:text-red-400">{{ t('dashboard.chartsLoadFailed') }}</p>
          <button class="btn btn-secondary" :disabled="loadingCharts" @click="loadCharts">{{ t('common.retry') }}</button>
        </div>
        <UserDashboardCharts v-else v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2">
            <div v-if="loadErrors.usage" role="alert" class="card flex items-center justify-between gap-4 p-5">
              <p class="text-sm text-red-700 dark:text-red-400">{{ t('usage.failedToLoad') }}</p>
              <button class="btn btn-secondary" :disabled="loadingUsage" @click="loadRecent">{{ t('common.retry') }}</button>
            </div>
            <UserDashboardRecentUsage v-else :data="recentUsage" :loading="loadingUsage" />
          </div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'; import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { formatDateLocalInput } from '@/utils/format'

const authStore = useAuthStore(); const user = computed(() => authStore.user)
const { t } = useI18n()
const loadErrors = reactive({ stats: false, charts: false, usage: false, quotas: false })
const stats = ref<UserStatsType | null>(null); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)

const today = formatDateLocalInput(new Date())
const startDate = ref(today); const endDate = ref(today); const granularity = ref<'hour' | 'day'>('hour')

const loadStats = async () => {
  loading.value = true
  loadErrors.stats = false
  try {
    await authStore.refreshUser()
    stats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    loadErrors.stats = true
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}
const loadCharts = async () => {
  loadingCharts.value = true
  loadErrors.charts = false
  try {
    const [trend, models] = await Promise.all([
      usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value }),
      usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })
    ])
    trendData.value = trend.trend
    modelStats.value = models.models
  } catch (error) {
    loadErrors.charts = true
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}
const loadRecent = async () => {
  loadingUsage.value = true
  loadErrors.usage = false
  try {
    const response = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = response.items.slice(0, 5)
  } catch (error) {
    loadErrors.usage = true
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}
const loadPlatformQuotas = async () => {
  loadErrors.quotas = false
  try {
    const data = await getMyPlatformQuotas()
    platformQuotas.value = data.platform_quotas ?? []
  } catch (error) {
    loadErrors.quotas = true
    console.error('Failed to load platform quotas:', error)
  }
}
const refreshAll = () => { loadStats(); loadCharts(); loadRecent(); loadPlatformQuotas() }

onMounted(() => { refreshAll() })
</script>
