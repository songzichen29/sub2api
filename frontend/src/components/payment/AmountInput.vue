<template>
  <div class="space-y-4">
    <!-- Quick Amount Buttons -->
    <div>
      <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.quickAmounts') }}
      </label>
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :class="[
            'flex min-h-[86px] flex-col justify-center rounded-lg border-2 px-4 py-3 text-center transition-colors',
            modelValue === amt
              ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/40 dark:text-primary-300'
              : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
          ]"
          @click="selectAmount(amt)"
        >
          <span class="flex items-center justify-center gap-2 font-semibold">
            <span>{{ formatAmountValue(amt) }}</span>
            <span
              v-if="amountPreview(amt).discountLabel"
              class="rounded-md bg-green-100 px-1.5 py-0.5 text-xs font-semibold text-green-700 dark:bg-green-900/40 dark:text-green-300"
            >
              {{ amountPreview(amt).discountLabel }}
            </span>
          </span>
          <span
            v-if="showAmountPreview"
            class="mt-2 flex flex-wrap justify-center gap-x-2 gap-y-1 text-xs font-normal text-gray-500 dark:text-gray-400"
          >
            <span>{{ t('payment.quickAmountPay', { amount: formatMoney(amountPreview(amt).payAmount) }) }}</span>
            <span>{{ t('payment.quickAmountSave', { amount: formatMoney(amountPreview(amt).discountAmount) }) }}</span>
          </span>
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div>
      <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.customAmount') }}
      </label>
      <div class="relative">
        <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500">
          ￥
        </span>
        <input
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          class="input w-full py-3 pl-8 pr-4"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { DiscountRule } from '@/types/payment'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  discountRules?: DiscountRule[]
  feeRate?: number
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  discountRules: () => [],
  feeRate: 0,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t, locale } = useI18n()

const customText = ref('')

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const enabledDiscountRules = computed(() =>
  props.discountRules
    .filter((rule) => rule.enabled !== false && rule.threshold > 0 && rule.value > 0)
    .sort((a, b) => a.threshold - b.threshold)
)

const showAmountPreview = computed(() => enabledDiscountRules.value.length > 0 || props.feeRate > 0)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function roundMoney(value: number): number {
  return Math.round((Number.isFinite(value) ? value : 0) * 100) / 100
}

function formatAmountValue(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2)
}

function formatMoney(value: number): string {
  return formatPaymentAmount(roundMoney(value), 'CNY', locale.value)
}

function findAppliedRule(amount: number): DiscountRule | null {
  let applied: DiscountRule | null = null
  for (const rule of enabledDiscountRules.value) {
    if (amount + 1e-9 >= rule.threshold) applied = rule
  }
  return applied
}

function discountLabel(rule: DiscountRule | null): string {
  if (!rule) return ''
  if (rule.label) return rule.label
  if (rule.type === 'rate') return t('payment.thresholdDiscountBadge', { rate: Math.round(rule.value * 100) })
  return t('payment.thresholdReduceBadge', { amount: formatAmountValue(rule.value) })
}

function amountPreview(amount: number): { discountAmount: number; payAmount: number; discountLabel: string } {
  const base = roundMoney(amount)
  const applied = findAppliedRule(base)
  let discount = 0
  if (applied?.type === 'rate') {
    discount = base * (1 - applied.value)
  } else if (applied?.type === 'reduce') {
    discount = applied.value
  }
  discount = Math.min(base, Math.max(0, roundMoney(discount)))
  const afterDiscount = roundMoney(base - discount)
  const fee = props.feeRate > 0 ? Math.ceil(((afterDiscount * props.feeRate) / 100) * 100) / 100 : 0
  return {
    discountAmount: discount,
    payAmount: roundMoney(afterDiscount + fee),
    discountLabel: discountLabel(applied),
  }
}

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  if (!AMOUNT_PATTERN.test(val)) return
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
