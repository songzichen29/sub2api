<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Subscriptions Grid -->
      <div v-else class="grid gap-6 lg:grid-cols-2">
        <div
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="overflow-hidden rounded-2xl border bg-white dark:bg-dark-800"
          :class="platformBorderClass(subscription.group?.platform || '')"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div :class="['h-1.5 w-1.5 shrink-0 rounded-full', platformAccentDotClass(subscription.group?.platform || '')]" />
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                  </h3>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                    {{ platformLabel(subscription.group?.platform || '') }}
                  </span>
                </div>
                <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group.description }}
                </p>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  subscription.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                    : subscription.status === 'expired'
                      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                      : subscription.status === 'quota_exhausted'
                        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
                        : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                ]"
              >
                {{ t(`userSubscriptions.status.${subscription.status}`) }}
              </span>
              <button
                v-if="subscription.status === 'active' || subscription.status === 'quota_exhausted'"
                :class="['rounded-lg px-3 py-1.5 text-xs font-semibold text-white transition-colors', platformButtonClass(subscription.group?.platform || '')]"
                @click="startRenewal(subscription)"
              >
                {{ t('payment.renewNow') }}
              </button>
              <button
                v-if="canResetDailyLimit(subscription)"
                class="rounded-lg border border-gray-200 px-3 py-1.5 text-xs font-semibold text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
                @click="resetDailyLimit(subscription)"
              >
                {{ t('userSubscriptions.resetDailyLimit') }}
              </button>
            </div>
          </div>

          <!-- Usage Progress -->
          <div class="space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <div
              v-if="canConfigureDailyOverdraft(subscription)"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
            >
              <label class="flex items-start justify-between gap-3">
                <span>
                  <span class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('userSubscriptions.dailyOverdraft') }}
                  </span>
                  <span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">
                    {{ t('userSubscriptions.dailyOverdraftHint') }}
                  </span>
                </span>
                <input
                  type="checkbox"
                  class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:opacity-60"
                  :checked="subscription.allow_daily_overdraft"
                  :disabled="overdraftUpdatingId === subscription.id"
                  @change="toggleDailyOverdraft(subscription, ($event.target as HTMLInputElement).checked)"
                />
              </label>
            </div>

            <div
              v-if="canConfigureWeekendSkip(subscription) || subscription.skip_weekends || subscription.weekend_skip_user_changed_at"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
            >
              <label class="flex items-start justify-between gap-3">
                <span>
                  <span class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                    跳过非工作日
                  </span>
                  <span class="mt-1 block text-xs text-gray-500 dark:text-dark-400">
                    开启后周六、周日不可使用，系统会自动顺延到期时间。此设置只能自行开启一次。
                  </span>
                </span>
                <input
                  type="checkbox"
                  class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:opacity-60"
                  :checked="subscription.skip_weekends"
                  :disabled="!canConfigureWeekendSkip(subscription) || weekendSkipUpdatingId === subscription.id"
                  @change="toggleWeekendSkip(subscription, ($event.target as HTMLInputElement).checked)"
                />
              </label>
              <p v-if="subscription.weekend_skip_user_changed_at && !subscription.skip_weekends" class="mt-2 text-xs text-gray-500 dark:text-dark-400">
                自助修改机会已使用，如需再次调整请联系管理员。
              </p>
            </div>

            <!-- Total Quota -->
            <div v-if="hasTotalQuota(subscription)" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.totalQuota') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ getTotalQuotaUsed(subscription).toFixed(2) }} / ${{ getTotalQuotaLimit(subscription).toFixed(2) }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="getProgressBarClass(getTotalQuotaUsed(subscription), getTotalQuotaLimit(subscription))"
                  :style="{ width: getProgressWidth(getTotalQuotaUsed(subscription), getTotalQuotaLimit(subscription)) }"
                ></div>
              </div>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('userSubscriptions.totalQuotaRemaining', { amount: getTotalQuotaRemaining(subscription).toFixed(2) }) }}
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="getOverdraftLimit(subscription) || subscription.group?.weekly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ getOverdraftLimit(subscription) ? t('userSubscriptions.overdraftTotal') : t('userSubscriptions.weekly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ ((getOverdraftDisplayUsed(subscription) ?? subscription.weekly_usage_usd) || 0).toFixed(2) }} / ${{
                    (getOverdraftLimit(subscription) || subscription.group!.weekly_limit_usd!).toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      getOverdraftDisplayUsed(subscription) ?? subscription.weekly_usage_usd,
                      getOverdraftLimit(subscription) || subscription.group?.weekly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      getOverdraftDisplayUsed(subscription) ?? subscription.weekly_usage_usd,
                      getOverdraftLimit(subscription) || subscription.group?.weekly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="!getOverdraftLimit(subscription) && subscription.weekly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.weekly_window_start, 168)
                  })
                }}
              </p>
              <div v-if="getOverdraftLimit(subscription)" class="grid gap-2 rounded-lg bg-gray-50 p-2 text-xs dark:bg-dark-700/50 sm:grid-cols-2">
                <div class="flex items-center justify-between gap-2 text-gray-600 dark:text-dark-300">
                  <span>{{ t('userSubscriptions.todayOverdraft') }}</span>
                  <span class="font-medium text-gray-800 dark:text-gray-100">${{ getTodayOverdraftAmount(subscription).toFixed(2) }}</span>
                </div>
                <div class="flex items-center justify-between gap-2 text-gray-600 dark:text-dark-300">
                  <span>{{ t('userSubscriptions.overdraftRemaining') }}</span>
                  <span class="font-medium text-gray-800 dark:text-gray-100">${{ getOverdraftRemaining(subscription).toFixed(2) }}</span>
                </div>
              </div>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd && !getOverdraftLimit(subscription)" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.daily') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.daily_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.daily_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.daily_window_start, 24)
                  })
                }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="!getOverdraftLimit(subscription) && subscription.group?.monthly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.monthly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.monthly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.monthly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.monthly_window_start, 720)
                  })
                }}
              </p>
            </div>

            <!-- No limits configured - Unlimited badge -->
            <div
              v-if="
                !subscription.group?.daily_limit_usd &&
                !subscription.group?.weekly_limit_usd &&
                !subscription.group?.monthly_limit_usd &&
                !getOverdraftLimit(subscription) &&
                !hasTotalQuota(subscription)
              "
              class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-teal-50 py-6 dark:from-emerald-900/20 dark:to-teal-900/20"
            >
              <div class="flex items-center gap-3">
                <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                <div>
                  <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    {{ t('userSubscriptions.unlimited') }}
                  </p>
                  <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                    {{ t('userSubscriptions.unlimitedDesc') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <ConfirmDialog
      :show="!!renewalTarget"
      :title="t('payment.renewalNoticeTitle')"
      :message="renewalTarget ? buildRenewalNotice(renewalTarget) : ''"
      :confirm-text="t('payment.continueRenewal')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmRenewal"
      @cancel="renewalTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI from '@/api/subscriptions'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateOnly, formatRemainingDuration } from '@/utils/format'
import { platformBorderClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import { buildRenewalNoticeText } from './renewalNotice'

function platformAccentDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
const loading = ref(true)
const overdraftUpdatingId = ref<number | null>(null)
const weekendSkipUpdatingId = ref<number | null>(null)
const renewalTarget = ref<UserSubscription | null>(null)

async function loadSubscriptions() {
  try {
    loading.value = true
    subscriptions.value = await subscriptionsAPI.getMySubscriptions()
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function hasTotalQuota(subscription: UserSubscription): boolean {
  return subscription.quota_limit_usd != null && subscription.quota_limit_usd > 0
}

function getTotalQuotaLimit(subscription: UserSubscription): number {
  return subscription.quota_limit_usd && subscription.quota_limit_usd > 0 ? subscription.quota_limit_usd : 0
}

function getTotalQuotaUsed(subscription: UserSubscription): number {
  return Math.max(subscription.quota_used_usd || 0, 0)
}

function getTotalQuotaRemaining(subscription: UserSubscription): number {
  if (typeof subscription.quota_remaining_usd === 'number') {
    return Math.max(subscription.quota_remaining_usd, 0)
  }
  return Math.max(getTotalQuotaLimit(subscription) - getTotalQuotaUsed(subscription), 0)
}

function getOverdraftLimit(subscription: UserSubscription): number | null {
  return subscription.allow_daily_overdraft
    && typeof subscription.overdraft_limit_usd === 'number'
    && subscription.overdraft_limit_usd > 0
    ? subscription.overdraft_limit_usd
    : null
}

function getOverdraftDisplayUsed(subscription: UserSubscription): number | null {
  if (getOverdraftLimit(subscription) === null) return null
  if (typeof subscription.overdraft_used_usd === 'number') {
    return Math.max(subscription.overdraft_used_usd, 0)
  }
  return isDayValidityUnit(subscription.validity_unit)
    ? getDayValidityOverdraftUsed(subscription)
    : (subscription.weekly_usage_usd ?? 0)
}

function isDayValidityUnit(unit?: string | null): boolean {
  const normalized = (unit || 'day').trim().toLowerCase()
  return normalized === '' || normalized === 'day' || normalized === 'days'
}

function getTodayOverdraftAmount(subscription: UserSubscription): number {
  const dailyLimit = subscription.group?.daily_limit_usd
  if (!dailyLimit || dailyLimit <= 0 || getOverdraftLimit(subscription) === null) return 0
  return Math.max((subscription.daily_usage_usd || 0) - dailyLimit, 0)
}

function getElapsedOverdraftQuota(subscription: UserSubscription): number {
  const dailyLimit = subscription.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(subscription)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  return Math.min(dailyLimit * getElapsedFullOverdraftDays(subscription), overdraftLimit)
}

function getDayValidityOverdraftUsed(subscription: UserSubscription): number {
  const dailyLimit = subscription.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(subscription)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  const expiredQuota = getElapsedOverdraftQuota(subscription)
  const currentDailyUsage = getCurrentDailyWindowUsage(subscription)
  const effectiveUsed = expiredQuota + currentDailyUsage
  const actualUsed = subscription.weekly_usage_usd ?? subscription.overdraft_used_usd ?? 0
  return Math.min(Math.max(actualUsed, effectiveUsed), overdraftLimit)
}

function getElapsedFullOverdraftDays(subscription: UserSubscription): number {
  const dailyLimit = subscription.group?.daily_limit_usd
  const overdraftLimit = getOverdraftLimit(subscription)
  if (!dailyLimit || dailyLimit <= 0 || overdraftLimit === null) return 0

  const startsAt = subscription.starts_at ? new Date(subscription.starts_at).getTime() : NaN
  if (!Number.isFinite(startsAt)) return 0

  const dayMs = 24 * 60 * 60 * 1000
  const elapsedDays = Math.max(0, Math.floor((Date.now() - startsAt) / dayMs))
  const validityDays = Math.max(1, Math.ceil(overdraftLimit / dailyLimit))
  return Math.min(elapsedDays, validityDays)
}

function getCurrentDailyWindowUsage(subscription: UserSubscription): number {
  if (!subscription.daily_window_start || !subscription.starts_at) return 0

  const startsAt = new Date(subscription.starts_at).getTime()
  const dailyWindowStart = new Date(subscription.daily_window_start).getTime()
  if (!Number.isFinite(startsAt) || !Number.isFinite(dailyWindowStart)) return 0

  const dayMs = 24 * 60 * 60 * 1000
  const elapsedDays = Math.max(0, Math.floor((Date.now() - startsAt) / dayMs))
  const currentWindowStart = startsAt + elapsedDays * dayMs
  if (dailyWindowStart !== currentWindowStart) return 0

  return Math.max(subscription.daily_usage_usd || 0, 0)
}

function getOverdraftRemaining(subscription: UserSubscription): number {
  const limit = getOverdraftLimit(subscription)
  if (limit === null) return 0
  return Math.max(limit - (getOverdraftDisplayUsed(subscription) ?? 0), 0)
}

function canConfigureDailyOverdraft(subscription: UserSubscription): boolean {
  return subscription.status === 'active'
    && !!subscription.group?.allow_daily_overdraft
    && !!subscription.group?.daily_limit_usd
    && subscription.group.daily_limit_usd > 0
}

function canConfigureWeekendSkip(subscription: UserSubscription): boolean {
  return subscription.status === 'active'
    && !!subscription.group?.allow_weekend_skip
    && !subscription.skip_weekends
    && !subscription.weekend_skip_user_changed_at
}

async function toggleDailyOverdraft(subscription: UserSubscription, enabled: boolean) {
  if (!canConfigureDailyOverdraft(subscription)) return
  const previous = subscription.allow_daily_overdraft
  subscription.allow_daily_overdraft = enabled
  overdraftUpdatingId.value = subscription.id
  try {
    const updated = await subscriptionsAPI.setDailyOverdraft(subscription.id, enabled)
    Object.assign(subscription, updated)
    appStore.showSuccess(
      enabled
        ? t('userSubscriptions.dailyOverdraftEnabled')
        : t('userSubscriptions.dailyOverdraftDisabled')
    )
  } catch (error) {
    subscription.allow_daily_overdraft = previous
    console.error('Failed to update daily overdraft:', error)
    appStore.showError(t('userSubscriptions.dailyOverdraftUpdateFailed'))
  } finally {
    overdraftUpdatingId.value = null
  }
}

async function toggleWeekendSkip(subscription: UserSubscription, enabled: boolean) {
  if (!enabled) {
    subscription.skip_weekends = true
    appStore.showError('跳过非工作日开启后不能自行关闭，请联系管理员。')
    return
  }
  if (!canConfigureWeekendSkip(subscription)) return
  const previous = subscription.skip_weekends
  subscription.skip_weekends = enabled
  weekendSkipUpdatingId.value = subscription.id
  try {
    const updated = await subscriptionsAPI.setWeekendSkip(subscription.id, enabled)
    Object.assign(subscription, updated)
    appStore.showSuccess('已开启跳过非工作日，到期时间已自动顺延')
  } catch (error) {
    subscription.skip_weekends = previous
    console.error('Failed to update weekend skip:', error)
    appStore.showError('跳过非工作日设置更新失败')
  } finally {
    weekendSkipUpdatingId.value = null
  }
}

function canResetDailyLimit(subscription: UserSubscription): boolean {
  return subscription.status === 'active'
    && !!subscription.group?.daily_limit_usd
    && subscription.group.daily_limit_usd > 0
    && !!subscription.group.daily_limit_reset_price
    && subscription.group.daily_limit_reset_price > 0
}

function resetDailyLimit(subscription: UserSubscription) {
  router.push({
    path: '/purchase',
    query: {
      tab: 'daily_limit_reset',
      subscription_id: String(subscription.id),
    },
  })
}

function getQuotaRemaining(subscription: UserSubscription, key: 'daily_limit_usd' | 'weekly_limit_usd' | 'monthly_limit_usd', used: number): number | null {
  const limit = subscription.group?.[key]
  if (limit == null || limit <= 0) return null
  return Math.max(limit - (used || 0), 0)
}

function subscriptionHasRenewalWarning(subscription: UserSubscription): boolean {
  if (hasTotalQuota(subscription) && getTotalQuotaRemaining(subscription) > 0) return true
  const group = subscription.group
  if (!group) return true
  const overdraftLimit = getOverdraftLimit(subscription)
  if (overdraftLimit !== null) {
    return (overdraftLimit - (getOverdraftDisplayUsed(subscription) ?? 0)) > 0
  }
  const hasUnlimitedQuota = (group.daily_limit_usd == null || group.daily_limit_usd <= 0)
    && (group.weekly_limit_usd == null || group.weekly_limit_usd <= 0)
    && (group.monthly_limit_usd == null || group.monthly_limit_usd <= 0)
    && !hasTotalQuota(subscription)
  if (hasUnlimitedQuota) return true
  return (getQuotaRemaining(subscription, 'daily_limit_usd', subscription.daily_usage_usd) ?? 0) > 0
    || (getQuotaRemaining(subscription, 'weekly_limit_usd', subscription.weekly_usage_usd) ?? 0) > 0
    || (getQuotaRemaining(subscription, 'monthly_limit_usd', subscription.monthly_usage_usd) ?? 0) > 0
}

function buildRenewalNotice(subscription: UserSubscription): string {
  return buildRenewalNoticeText(subscription, t, subscription.validity_days ?? null)
}

function goRenewal(subscription: UserSubscription) {
  router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })
}

function startRenewal(subscription: UserSubscription) {
  if (!subscriptionHasRenewalWarning(subscription)) {
    goRenewal(subscription)
    return
  }
  renewalTarget.value = subscription
}

function confirmRenewal() {
  if (!renewalTarget.value) return
  const target = renewalTarget.value
  renewalTarget.value = null
  goRenewal(target)
}

function formatExpirationDate(expiresAt: string): string {
  const expires = new Date(expiresAt)
  const remaining = formatRemainingDuration(expiresAt)
  if (!remaining) {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateOnly(expires)
  return `${t('userSubscriptions.exactRemaining', { time: remaining })} (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (diff <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatResetTime(windowStart: string | null, windowHours: number): string {
  if (!windowStart) return t('userSubscriptions.windowNotActive')

  const start = new Date(windowStart)
  const end = new Date(start.getTime() + windowHours * 60 * 60 * 1000)
  const now = new Date()
  const diff = end.getTime() - now.getTime()

  if (diff <= 0) return t('userSubscriptions.windowNotActive')

  const hours = Math.floor(diff / (1000 * 60 * 60))
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))

  if (hours > 24) {
    const days = Math.floor(hours / 24)
    const remainingHours = hours % 24
    return `${days}d ${remainingHours}h`
  }

  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }

  return `${minutes}m`
}

onMounted(() => {
  loadSubscriptions()
})
</script>
