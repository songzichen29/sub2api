<template>
  <div
    :class="[
      'group relative flex flex-col overflow-hidden rounded-2xl border transition-all',
      'hover:shadow-xl hover:-translate-y-0.5',
      borderClass,
      'bg-white dark:bg-dark-800',
    ]"
  >
    <!-- Colored top accent bar -->
    <div :class="['h-1.5', accentClass]" />

    <div class="flex flex-1 flex-col p-4">
      <!-- Header: name + badge + price -->
      <div class="mb-3 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
            <span :class="['shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', badgeLightClass]">
              {{ pLabel }}
            </span>
          </div>
          <p v-if="plan.description" class="mt-0.5 text-xs leading-relaxed text-gray-500 dark:text-dark-400 line-clamp-2">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div v-if="typeof plan.sales_count === 'number'" class="mb-1 text-[11px] text-gray-400 dark:text-dark-500">
            {{ t('payment.planCard.salesCount', { count: plan.sales_count }) }}
          </div>
          <div class="flex items-baseline gap-1">
            <span class="text-xs text-gray-400 dark:text-dark-500">{{ planCurrencySymbol }}</span>
            <span :class="['text-2xl font-extrabold tracking-tight', textClass]">{{ plan.price }}</span>
            <span v-if="plan.currency" class="text-xs font-medium text-gray-400 dark:text-dark-500">{{ plan.currency }}</span>
          </div>
          <span class="text-[11px] text-gray-400 dark:text-dark-500">/ {{ validitySuffix }}</span>
          <div v-if="plan.original_price" class="mt-0.5 flex items-center justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">{{ planCurrencySymbol }}{{ plan.original_price }}{{ plan.currency || '' }}</span>
            <span :class="['rounded px-1 py-0.5 text-[10px] font-semibold', discountClass]">{{ discountText }}</span>
          </div>
        </div>
      </div>

      <!-- Purchase limit badge -->
    <div v-if="plan.max_buy_count && plan.max_buy_count > 0" class="mb-2 flex items-center gap-1.5">
      <span :class="[
        'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium',
        purchaseLimitReached
          ? 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-400'
          : 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-400'
      ]">
        {{ purchaseLimitReached
          ? t('payment.planCard.purchaseLimitReached')
          : t('payment.planCard.purchaseLimit', { remaining: plan.max_buy_count - (plan.user_buy_count || 0), total: plan.max_buy_count })
        }}
      </span>
    </div>

    <!-- Group quota info (compact) -->
      <div class="mb-3 grid grid-cols-2 gap-x-3 gap-y-1 rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-700/50">
        <div class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.rate') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ rateDisplay }}</span>
        </div>
        <div v-if="hasTotalQuota" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.totalQuota') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ formatUSD(plan.quota_limit_usd) }}</span>
        </div>
        <div v-if="plan.daily_limit_usd != null && plan.daily_limit_usd > 0" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ formatUSD(plan.daily_limit_usd) }}</span>
        </div>
        <div v-if="overdraftPoolLimit !== null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.overdraftTotal') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ formatUSD(overdraftPoolLimit) }}</span>
        </div>
        <div v-else-if="plan.weekly_limit_usd != null && plan.weekly_limit_usd > 0" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ formatUSD(plan.weekly_limit_usd) }}</span>
        </div>
        <div v-else-if="plan.monthly_limit_usd != null && plan.monthly_limit_usd > 0" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ formatUSD(plan.monthly_limit_usd) }}</span>
        </div>
        <div v-if="!hasAnyQuotaLimit" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="col-span-2 flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.models') }}</span>
          <div class="flex flex-wrap justify-end gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded bg-gray-200/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <!-- Features list (compact) -->
      <div v-if="plan.features.length > 0" class="mb-3 space-y-1">
        <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-1.5">
          <svg :class="['mt-0.5 h-3.5 w-3.5 flex-shrink-0', iconClass]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="text-xs text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <!-- Subscribe Button -->
      <button
        type="button"
        :disabled="purchaseLimitReached"
        :class="[
          'w-full rounded-xl py-2.5 text-sm font-semibold transition-all',
          purchaseLimitReached
            ? 'cursor-not-allowed bg-gray-200 text-gray-400 dark:bg-dark-600 dark:text-dark-400'
            : btnClass + ' active:scale-[0.98]'
        ]"
        @click="!purchaseLimitReached && emit('select', plan)"
      >
        {{ purchaseLimitReached ? t('payment.planCard.soldOut') : isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan } from '@/types/payment'
import type { UserSubscription } from '@/types'
import { currencySymbol } from '@/components/payment/currency'
import { planValiditySuffix } from './validity'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformTextClass,
  platformIconClass,
  platformButtonClass,
  platformDiscountClass,
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[] }>()
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')
const isRenewal = computed(() =>
  props.activeSubscriptions?.some(s => s.group_id === props.plan.group_id && s.status === 'active') ?? false
)
const purchaseLimitReached = computed(() => {
  const limit = props.plan.max_buy_count
  if (!limit || limit <= 0) return false
  return (props.plan.user_buy_count || 0) >= limit
})

// Derived color classes from central config
const accentClass = computed(() => platformAccentBarClass(platform.value))
const borderClass = computed(() => platformBorderClass(platform.value))
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const textClass = computed(() => platformTextClass(platform.value))
const iconClass = computed(() => platformIconClass(platform.value))
const btnClass = computed(() => platformButtonClass(platform.value))
const discountClass = computed(() => platformDiscountClass(platform.value))
const pLabel = computed(() => platformLabel(platform.value))
const planCurrencySymbol = computed(() => currencySymbol(props.plan.currency || 'USD'))

const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `×${Number(rate.toPrecision(10))}`
})

const hasTotalQuota = computed(() =>
  props.plan.quota_limit_usd != null && props.plan.quota_limit_usd > 0
)

const hasAnyQuotaLimit = computed(() =>
  hasTotalQuota.value ||
  (props.plan.daily_limit_usd != null && props.plan.daily_limit_usd > 0) ||
  (props.plan.weekly_limit_usd != null && props.plan.weekly_limit_usd > 0) ||
  (props.plan.monthly_limit_usd != null && props.plan.monthly_limit_usd > 0) ||
  overdraftPoolLimit.value !== null
)

function formatUSD(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '0.00'
  return value.toFixed(2)
}

const overdraftPoolLimit = computed(() => {
  if (!props.plan.allow_daily_overdraft || props.plan.daily_limit_usd == null || props.plan.daily_limit_usd <= 0) {
    return null
  }
  const days = planValidityDays.value
  if (days <= 0) return null
  return props.plan.daily_limit_usd * days
})

const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const modelScopeLabels = computed(() => {
  if (platform.value !== 'antigravity') return []
  const scopes = props.plan.supported_model_scopes
  if (!scopes || scopes.length === 0) return []
  return scopes.map(s => MODEL_SCOPE_LABELS[s] || s)
})

const fixedExpiresAtText = computed(() => {
  if (!props.plan.expires_at) return ''
  const date = new Date(props.plan.expires_at)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString()
})

const planValidityDays = computed(() => {
  if (props.plan.expires_at) {
    const expiresAt = new Date(props.plan.expires_at)
    const diffMs = expiresAt.getTime() - Date.now()
    if (!Number.isNaN(diffMs) && diffMs > 0) {
      return Math.max(1, Math.ceil(diffMs / (24 * 60 * 60 * 1000)))
    }
    return 0
  }
  const unit = (props.plan.validity_unit || 'day').trim().toLowerCase()
  let days = props.plan.validity_days || 0
  if (unit === 'week' || unit === 'weeks') days *= 7
  if (unit === 'month' || unit === 'months') days *= 30
  return days
})

const validitySuffix = computed(() => {
  if (fixedExpiresAtText.value) {
    return t('payment.planCard.untilDate', { date: fixedExpiresAtText.value })
  }
  return planValiditySuffix(props.plan, t)
})
</script>
