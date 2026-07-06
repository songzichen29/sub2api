<template>
  <div class="space-y-2 text-sm">
    <div class="flex justify-between">
      <span class="text-gray-500 dark:text-gray-400">{{ baseLabel }}</span>
      <span class="text-gray-900 dark:text-white">{{ money(baseAmount) }}</span>
    </div>
    <div v-if="thresholdDiscount > 0" class="flex justify-between">
      <span class="text-gray-500 dark:text-gray-400">{{ thresholdLabel }}</span>
      <span class="text-green-600 dark:text-green-400">-{{ money(thresholdDiscount) }}</span>
    </div>
    <div v-if="couponDiscount > 0" class="flex justify-between">
      <span class="text-gray-500 dark:text-gray-400">{{ couponLabel }}</span>
      <span class="text-green-600 dark:text-green-400">-{{ money(couponDiscount) }}</span>
    </div>
    <div v-if="fee > 0" class="flex justify-between">
      <span class="text-gray-500 dark:text-gray-400">{{ feeLabel }}{{ feeRate > 0 ? ` (${feeRate}%)` : '' }}</span>
      <span class="text-gray-900 dark:text-white">{{ money(fee) }}</span>
    </div>
    <div class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
      <span class="font-medium text-gray-700 dark:text-gray-300">{{ payLabel }}</span>
      <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ money(payAmount) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { formatPaymentAmount } from '@/components/payment/currency'

const props = withDefaults(defineProps<{
  baseAmount: number
  thresholdDiscount?: number
  couponDiscount?: number
  fee?: number
  payAmount: number
  feeRate?: number
  currency?: string
  locale?: string
  baseLabel?: string
  thresholdLabel?: string
  couponLabel?: string
  feeLabel?: string
  payLabel?: string
}>(), {
  thresholdDiscount: 0,
  couponDiscount: 0,
  fee: 0,
  feeRate: 0,
  currency: 'CNY',
  locale: undefined,
  baseLabel: '原价',
  thresholdLabel: '满减',
  couponLabel: '优惠券',
  feeLabel: '手续费',
  payLabel: '实付',
})

function money(value: number): string {
  return formatPaymentAmount(Number.isFinite(value) ? value : 0, props.currency || 'CNY', props.locale)
}
</script>
