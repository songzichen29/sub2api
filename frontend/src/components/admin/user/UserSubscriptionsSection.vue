<template>
  <section class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <span class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.users.subscriptionsTitle') }}
        </span>
        <span
          class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
        >
          {{ subscriptions.length }}
        </span>
      </div>
      <button
        v-if="subscriptions.length > 0"
        type="button"
        class="text-xs text-gray-500 transition-colors hover:text-primary-500 dark:text-dark-400 dark:hover:text-primary-400"
        @click="expanded = !expanded"
      >
        {{ expanded ? t('common.collapse') : t('common.expand') }}
      </button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex justify-center py-6">
      <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path
          class="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
        />
      </svg>
    </div>

    <!-- Empty -->
    <p
      v-else-if="subscriptions.length === 0"
      class="mt-2 text-xs text-gray-500 dark:text-dark-400"
    >
      {{ t('admin.users.noSubscriptionsForUser') }}
    </p>

    <!-- List -->
    <div
      v-else-if="expanded"
      class="mt-3 max-h-80 space-y-3 overflow-y-auto pr-1"
    >
      <div
        v-for="sub in subscriptions"
        :key="sub.id"
        class="rounded-lg border border-gray-200 bg-gray-50/50 p-3 dark:border-dark-600 dark:bg-dark-700/40"
      >
        <!-- Row 1: group + status + expires -->
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex min-w-0 items-center gap-2">
            <GroupBadge
              v-if="sub.group"
              :name="sub.group.name"
              :platform="sub.group.platform"
              :subscription-type="sub.group.subscription_type"
              :rate-multiplier="sub.group.rate_multiplier"
              :days-remaining="sub.expires_at ? getDaysRemaining(sub.expires_at) : null"
              :remaining-text="sub.expires_at ? getRemainingText(sub.expires_at) : null"
              :show-rate="false"
            />
            <span v-else class="text-xs text-gray-400 dark:text-dark-500">-</span>
            <span
              :class="[
                'badge text-[11px]',
                isSubscriptionNotStarted(sub)
                  ? 'badge-warning'
                  : sub.status === 'active'
                    ? 'badge-success'
                    : sub.status === 'expired'
                      ? 'badge-warning'
                      : 'badge-danger'
              ]"
            >
              {{
                isSubscriptionNotStarted(sub)
                  ? t('admin.subscriptions.status.not_started')
                  : t(`admin.subscriptions.status.${sub.status}`)
              }}
            </span>
          </div>
          <div class="text-right text-xs">
            <div v-if="sub.expires_at" class="text-gray-700 dark:text-gray-300">
              <span :class="isExpiringSoon(sub.expires_at) ? 'text-orange-600 dark:text-orange-400' : ''">
                {{ formatDateOnly(sub.expires_at) }}
              </span>
              <span
                v-if="getDaysRemaining(sub.expires_at) !== null"
                class="ml-1 text-gray-500 dark:text-dark-400"
              >
                ({{ t('admin.subscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }})
              </span>
            </div>
            <div v-else class="text-gray-500 dark:text-dark-400">
              {{ t('admin.subscriptions.noExpiration') }}
            </div>
            <div v-if="sub.starts_at" class="text-[11px] text-gray-400 dark:text-dark-500">
              {{ t('admin.subscriptions.columns.startsAt') }}: {{ formatDateOnly(sub.starts_at) }}
            </div>
          </div>
        </div>

        <!-- Row 2: usage progress -->
        <div class="mt-3 space-y-2">
          <!-- Total Quota -->
          <div v-if="hasTotalQuota(sub)" class="usage-row">
            <div class="flex items-center gap-2">
              <span class="usage-label">{{ t('admin.subscriptions.totalQuota') }}</span>
              <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="h-1.5 rounded-full transition-all"
                  :class="getProgressClass(getTotalQuotaUsed(sub), getTotalQuotaLimit(sub))"
                  :style="{ width: getProgressWidth(getTotalQuotaUsed(sub), getTotalQuotaLimit(sub)) }"
                ></div>
              </div>
              <span class="usage-amount">
                ${{ getTotalQuotaUsed(sub).toFixed(2) }}
                <span class="text-gray-400">/</span>
                ${{ getTotalQuotaLimit(sub).toFixed(2) }}
              </span>
            </div>
            <div class="reset-info">
              <Icon name="dollar" size="xs" />
              <span>
                {{
                  t('admin.subscriptions.totalQuotaRemaining', {
                    amount: getTotalQuotaRemaining(sub).toFixed(2),
                  })
                }}
              </span>
            </div>
          </div>

          <!-- Daily -->
          <div v-if="sub.group?.daily_limit_usd" class="usage-row">
            <div class="flex items-center gap-2">
              <span class="usage-label">{{ t('admin.subscriptions.daily') }}</span>
              <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="h-1.5 rounded-full transition-all"
                  :class="getProgressClass(sub.daily_usage_usd, sub.group.daily_limit_usd)"
                  :style="{ width: getProgressWidth(sub.daily_usage_usd, sub.group.daily_limit_usd) }"
                ></div>
              </div>
              <span class="usage-amount">
                ${{ (sub.daily_usage_usd ?? 0).toFixed(2) }}
                <span class="text-gray-400">/</span>
                ${{ sub.group.daily_limit_usd.toFixed(2) }}
              </span>
            </div>
            <div v-if="sub.daily_window_start" class="reset-info">
              <Icon name="clock" size="xs" />
              <span>{{ formatResetTime(sub.daily_window_start, 'daily') }}</span>
            </div>
          </div>

          <!-- Overdraft total pool / Weekly -->
          <div v-if="getOverdraftLimit(sub) || sub.group?.weekly_limit_usd" class="usage-row">
            <div class="flex items-center gap-2">
              <span class="usage-label">
                {{ getOverdraftLimit(sub) ? t('admin.subscriptions.overdraftTotal') : t('admin.subscriptions.weekly') }}
              </span>
              <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="h-1.5 rounded-full transition-all"
                  :class="getProgressClass(getOverdraftDisplayUsed(sub) ?? sub.weekly_usage_usd, getOverdraftLimit(sub) || sub.group?.weekly_limit_usd || null)"
                  :style="{ width: getProgressWidth(getOverdraftDisplayUsed(sub) ?? sub.weekly_usage_usd, getOverdraftLimit(sub) || sub.group?.weekly_limit_usd || null) }"
                ></div>
              </div>
              <span class="usage-amount">
                ${{ ((getOverdraftDisplayUsed(sub) ?? sub.weekly_usage_usd) || 0).toFixed(2) }}
                <span class="text-gray-400">/</span>
                ${{ (getOverdraftLimit(sub) || sub.group!.weekly_limit_usd!).toFixed(2) }}
              </span>
            </div>
            <div v-if="!getOverdraftLimit(sub) && sub.weekly_window_start" class="reset-info">
              <Icon name="clock" size="xs" />
              <span>{{ formatResetTime(sub.weekly_window_start, 'weekly') }}</span>
            </div>
            <div v-if="sub.allow_daily_overdraft && getOverdraftLimit(sub)" class="reset-info">
              <Icon name="calendar" size="xs" />
              <span>
                {{
                  t('admin.subscriptions.todayOverdraftAmount', {
                    amount: getTodayOverdraftAmount(sub).toFixed(2),
                  })
                }}
              </span>
            </div>
            <div v-if="getOverdraftLimit(sub)" class="reset-info">
              <Icon name="dollar" size="xs" />
              <span>
                {{ t('admin.subscriptions.overdraftRemaining') }}:
                ${{ getOverdraftRemaining(sub).toFixed(2) }}
              </span>
            </div>
          </div>

          <!-- Monthly -->
          <div v-if="!getOverdraftLimit(sub) && sub.group?.monthly_limit_usd" class="usage-row">
            <div class="flex items-center gap-2">
              <span class="usage-label">{{ t('admin.subscriptions.monthly') }}</span>
              <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="h-1.5 rounded-full transition-all"
                  :class="getProgressClass(sub.monthly_usage_usd, sub.group.monthly_limit_usd)"
                  :style="{ width: getProgressWidth(sub.monthly_usage_usd, sub.group.monthly_limit_usd) }"
                ></div>
              </div>
              <span class="usage-amount">
                ${{ (sub.monthly_usage_usd ?? 0).toFixed(2) }}
                <span class="text-gray-400">/</span>
                ${{ sub.group.monthly_limit_usd.toFixed(2) }}
              </span>
            </div>
            <div v-if="sub.monthly_window_start" class="reset-info">
              <Icon name="clock" size="xs" />
              <span>{{ formatResetTime(sub.monthly_window_start, 'monthly') }}</span>
            </div>
          </div>

          <!-- Unlimited -->
          <div
            v-if="
              !sub.group?.daily_limit_usd &&
              !sub.group?.weekly_limit_usd &&
              !sub.group?.monthly_limit_usd &&
              !getOverdraftLimit(sub) &&
              !hasTotalQuota(sub)
            "
            class="flex items-center gap-2 rounded-md bg-gradient-to-r from-emerald-50 to-teal-50 px-2 py-1.5 dark:from-emerald-900/20 dark:to-teal-900/20"
          >
            <span class="text-base text-emerald-600 dark:text-emerald-400">∞</span>
            <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
              {{ t('admin.subscriptions.unlimited') }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { UserSubscription } from '@/types'
import { formatDateOnly, formatRemainingDuration } from '@/utils/format'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  /** 仅当 modal 打开时才加载，避免无谓请求 */
  show: boolean
  /** 用户 id；为 null 时不加载 */
  userId: number | null
}

const props = defineProps<Props>()
const { t } = useI18n()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
const loading = ref(false)
const expanded = ref(true)
let abortController: AbortController | null = null

/** 客户端按业务期望排序：active 优先，其次 expired，最后 revoked；同状态保留后端 created_at desc */
const statusWeight: Record<string, number> = {
  active: 0,
  expired: 1,
  revoked: 2
}

const sortByStatus = (items: UserSubscription[]): UserSubscription[] => {
  return [...items].sort((a, b) => {
    const wa = statusWeight[a.status] ?? 99
    const wb = statusWeight[b.status] ?? 99
    return wa - wb
  })
}

const loadSubscriptions = async (userId: number) => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  try {
    // 复用现有 list 接口（PaginatedResponse），page_size 给较大值覆盖单用户场景
    const res = await adminAPI.subscriptions.list(
      1,
      50,
      { user_id: userId, sort_by: 'created_at', sort_order: 'desc' },
      { signal: controller.signal }
    )
    if (controller.signal.aborted) return
    subscriptions.value = sortByStatus(res.items || [])
  } catch (error: any) {
    if (controller.signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    }
    console.error('Failed to load user subscriptions:', error)
    appStore.showError(t('admin.users.failedToLoadSubscriptions'))
    subscriptions.value = []
  } finally {
    if (abortController === controller) {
      loading.value = false
      abortController = null
    }
  }
}

watch(
  () => [props.show, props.userId] as const,
  ([show, userId]) => {
    if (!show || !userId) {
      // 关闭或无用户时清空，避免下次打开闪烁旧数据
      subscriptions.value = []
      return
    }
    void loadSubscriptions(userId)
  },
  { immediate: true }
)

// ===== Helpers (与 SubscriptionsView 同步实现) =====

const getDaysRemaining = (expiresAt: string): number | null => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

const getRemainingText = (expiresAt: string): string | null => formatRemainingDuration(expiresAt)

const isExpiringSoon = (expiresAt: string): boolean => {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
}

const isSubscriptionNotStarted = (sub: UserSubscription): boolean => {
  if (sub.status !== 'active' || !sub.starts_at) return false
  const startsAtMs = new Date(sub.starts_at).getTime()
  if (Number.isNaN(startsAtMs)) return false
  return startsAtMs > Date.now()
}

const hasTotalQuota = (sub: UserSubscription): boolean => {
  return sub.quota_limit_usd != null && sub.quota_limit_usd > 0
}

const getTotalQuotaLimit = (sub: UserSubscription): number => {
  return sub.quota_limit_usd && sub.quota_limit_usd > 0 ? sub.quota_limit_usd : 0
}

const getTotalQuotaUsed = (sub: UserSubscription): number => {
  return Math.max(sub.quota_used_usd || 0, 0)
}

const getTotalQuotaRemaining = (sub: UserSubscription): number => {
  if (typeof sub.quota_remaining_usd === 'number') {
    return Math.max(sub.quota_remaining_usd, 0)
  }
  return Math.max(getTotalQuotaLimit(sub) - getTotalQuotaUsed(sub), 0)
}

const getProgressWidth = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const usedValue = used ?? 0
  const percentage = Math.min((usedValue / limit) * 100, 100)
  return `${percentage}%`
}

const getProgressClass = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const usedValue = used ?? 0
  const percentage = (usedValue / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

const getOverdraftLimit = (sub: UserSubscription): number | null => {
  return typeof sub.overdraft_limit_usd === 'number'
    && sub.overdraft_limit_usd > 0
    ? sub.overdraft_limit_usd
    : null
}

const getOverdraftDisplayUsed = (sub: UserSubscription): number | null => {
  const limit = getOverdraftLimit(sub)
  if (limit === null) return null
  if (typeof sub.overdraft_used_usd === 'number') {
    return Math.min(Math.max(sub.overdraft_used_usd, 0), limit)
  }
  return isDayValidityUnit(sub.validity_unit)
    ? getDayValidityOverdraftUsed(sub)
    : (sub.overdraft_used_usd ?? sub.weekly_usage_usd ?? 0)
}

const isDayValidityUnit = (unit?: string | null): boolean => {
  const normalized = (unit || 'day').trim().toLowerCase()
  return normalized === '' || normalized === 'day' || normalized === 'days'
}

const getTodayOverdraftAmount = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  if (!dailyLimit || dailyLimit <= 0 || getOverdraftLimit(sub) === null) return 0
  return Math.max((sub.daily_usage_usd || 0) - dailyLimit, 0)
}

const getElapsedOverdraftQuota = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(sub)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  return Math.min(dailyLimit * getElapsedFullOverdraftDays(sub), overdraftLimit)
}

const getDayValidityOverdraftUsed = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(sub)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  const expiredQuota = getElapsedOverdraftQuota(sub)
  const currentDailyUsage = getCurrentDailyWindowUsage(sub)
  const effectiveUsed = expiredQuota + currentDailyUsage
  const actualUsed = sub.weekly_usage_usd ?? sub.overdraft_used_usd ?? 0
  return Math.min(Math.max(actualUsed, effectiveUsed), overdraftLimit)
}

const getElapsedFullOverdraftDays = (sub: UserSubscription): number => {
  const dailyLimit = sub.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(sub)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  const startsAt = sub.starts_at ? new Date(sub.starts_at).getTime() : NaN
  if (!Number.isFinite(startsAt)) return 0

  const dayMs = 24 * 60 * 60 * 1000
  const elapsedDays = Math.max(0, Math.floor((Date.now() - startsAt) / dayMs))
  const validityDays = Math.max(1, Math.ceil(overdraftLimit / dailyLimit))
  return Math.min(elapsedDays, validityDays)
}

const getCurrentDailyWindowUsage = (sub: UserSubscription): number => {
  if (!sub.daily_window_start || !sub.starts_at) return 0

  const startsAt = new Date(sub.starts_at).getTime()
  const dailyWindowStart = new Date(sub.daily_window_start).getTime()
  if (!Number.isFinite(startsAt) || !Number.isFinite(dailyWindowStart)) return 0

  const dayMs = 24 * 60 * 60 * 1000
  const elapsedDays = Math.max(0, Math.floor((Date.now() - startsAt) / dayMs))
  const currentWindowStart = startsAt + elapsedDays * dayMs
  if (dailyWindowStart !== currentWindowStart) return 0

  return Math.max(sub.daily_usage_usd || 0, 0)
}

const getOverdraftRemaining = (sub: UserSubscription): number => {
  const limit = getOverdraftLimit(sub)
  if (limit === null) return 0
  return Math.max(limit - (getOverdraftDisplayUsed(sub) ?? 0), 0)
}

const formatResetTime = (
  windowStart: string,
  period: 'daily' | 'weekly' | 'monthly'
): string => {
  if (!windowStart) return t('admin.subscriptions.windowNotActive')
  const start = new Date(windowStart)
  const now = new Date()

  let resetTime: Date
  switch (period) {
    case 'daily':
      resetTime = new Date(start.getTime() + 24 * 60 * 60 * 1000)
      break
    case 'weekly':
      resetTime = new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000)
      break
    case 'monthly':
      resetTime = new Date(start)
      resetTime.setMonth(resetTime.getMonth() + 1)
      break
  }

  if (resetTime <= now) return t('admin.subscriptions.windowNotActive')
  const diff = resetTime.getTime() - now.getTime()
  const days = Math.floor(diff / (24 * 60 * 60 * 1000))
  const hours = Math.floor((diff % (24 * 60 * 60 * 1000)) / (60 * 60 * 1000))
  const minutes = Math.floor((diff % (60 * 60 * 1000)) / (60 * 1000))
  if (days > 0) return t('admin.subscriptions.resetInDaysHours', { days, hours })
  if (hours > 0) return t('admin.subscriptions.resetInHoursMinutes', { hours, minutes })
  return t('admin.subscriptions.resetInMinutes', { minutes })
}
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
}

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.usage-amount {
  @apply whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-gray-300;
}

.reset-info {
  @apply flex items-center gap-1 pl-12 text-[10px] text-blue-600 dark:text-blue-400;
}
</style>
