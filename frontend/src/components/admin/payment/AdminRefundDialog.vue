<template>
  <BaseDialog
    :show="show"
    :title="t('payment.admin.refundOrder')"
    width="normal"
    @close="emit('cancel')"
  >
    <form id="refund-form" @submit.prevent="handleSubmit" class="space-y-4">
      <!-- Refund Request Info -->
      <div
        v-if="order?.refund_requested_at || order?.refund_request_reason"
        class="rounded-lg border border-violet-200 bg-violet-50 p-3 dark:border-violet-800 dark:bg-violet-900/20"
      >
        <div class="flex items-center gap-2 text-sm font-medium text-violet-700 dark:text-violet-300">
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          {{ t('payment.admin.refundRequestInfo') }}
        </div>
        <div v-if="order?.refund_requested_at" class="mt-2 flex justify-between text-sm">
          <span class="text-violet-600 dark:text-violet-400">{{ t('payment.admin.refundRequestedAt') }}</span>
          <span class="text-violet-800 dark:text-violet-200">{{ formatDateTime(order.refund_requested_at) }}</span>
        </div>
        <div v-if="order?.refund_request_reason" class="mt-1 text-sm">
          <span class="text-violet-600 dark:text-violet-400">{{ t('payment.admin.refundRequestReason') }}:</span>
          <span class="ml-1 text-violet-800 dark:text-violet-200">{{ order.refund_request_reason }}</span>
        </div>
      </div>

      <!-- Order Info -->
      <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
        <div class="flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</span>
          <span class="font-mono text-gray-900 dark:text-white">#{{ order?.id }}</span>
        </div>
        <div class="mt-1 flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.creditedAmount') }}</span>
          <span class="font-medium text-gray-900 dark:text-white">{{ order?.order_type === 'balance' ? '$' : '¥' }}{{ order?.amount?.toFixed(2) }}</span>
        </div>
        <div class="mt-1 flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</span>
          <span class="font-medium text-gray-900 dark:text-white">¥{{ order?.pay_amount?.toFixed(2) }}</span>
        </div>
        <div v-if="actuallyRefunded > 0" class="mt-1 flex justify-between text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.alreadyRefunded') }}</span>
          <span class="font-medium text-red-600 dark:text-red-400">{{ order?.order_type === 'balance' ? '$' : '¥' }}{{ actuallyRefunded.toFixed(2) }}</span>
        </div>
      </div>

      <!-- Deduct Balance -->
      <div>
        <div class="flex items-center gap-2">
          <input
            id="deduct-balance"
            v-model="form.deduct_balance"
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
          <label for="deduct-balance" class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('payment.admin.deductBalance') }}
          </label>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.deductBalanceHint') }}</span>
        </div>

        <!-- User Balance Info (when deduct_balance is checked) -->
        <div v-if="form.deduct_balance && userBalance != null" class="mt-3 grid grid-cols-2 gap-3">
          <div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700">
            <div class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.userBalance') }}</div>
            <div class="mt-1 font-semibold text-gray-900 dark:text-white">${{ userBalance.toFixed(2) }}</div>
          </div>
          <div class="rounded-lg bg-gray-50 p-3 text-sm dark:bg-dark-700">
            <div class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.orderAmount') }}</div>
            <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ order?.order_type === 'balance' ? '$' : '¥' }}{{ order?.amount?.toFixed(2) }}</div>
          </div>
        </div>

        <!-- Insufficient balance warning -->
        <div
          v-if="form.deduct_balance && balanceInsufficient"
          class="mt-2 rounded-lg bg-amber-50 p-3 text-sm text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {{ t('payment.admin.insufficientBalance') }}
        </div>

        <!-- No deduction info -->
        <div
          v-if="!form.deduct_balance"
          class="mt-2 rounded-lg bg-blue-50 p-3 text-sm text-blue-700 dark:bg-blue-900/20 dark:text-blue-300"
        >
          {{ t('payment.admin.noDeduction') }}
        </div>
      </div>

      <!-- Refund Amount -->
      <div v-if="isSubscriptionOrder" class="space-y-2">
        <label class="input-label">{{ t('payment.admin.refundMode') }}</label>
        <div class="space-y-2">
          <label class="flex items-start gap-3 rounded-md border border-gray-200 px-3 py-3 text-sm dark:border-dark-600">
            <input
              v-model="form.refund_mode"
              type="radio"
              value="proportional"
              class="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <div class="min-w-0">
              <div class="font-medium text-gray-900 dark:text-white">{{ t('payment.admin.proportionalRefund') }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.proportionalRefundHint') }}</div>
            </div>
          </label>
          <label class="flex items-start gap-3 rounded-md border border-gray-200 px-3 py-3 text-sm dark:border-dark-600">
            <input
              v-model="form.refund_mode"
              type="radio"
              value="full"
              class="mt-0.5 h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500"
            />
            <div class="min-w-0">
              <div class="font-medium text-gray-900 dark:text-white">{{ t('payment.admin.fullRefund') }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.fullRefundHint') }}</div>
            </div>
          </label>
        </div>
      </div>

      <!-- Refund Amount -->
      <div>
        <label class="input-label">{{ t('payment.admin.refundAmount') }}</label>
        <div class="relative">
          <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">{{ order?.order_type === 'balance' ? '$' : '¥' }}</span>
          <input
            v-model.number="form.amount"
            type="number"
            step="0.01"
            :min="isSubscriptionOrder && form.refund_mode === 'proportional' ? 0 : 0.01"
            :max="maxRefundable"
            class="input pl-7"
            :readonly="isSubscriptionOrder && form.refund_mode === 'proportional'"
            :required="!(isSubscriptionOrder && form.refund_mode === 'proportional')"
          />
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('payment.admin.maxRefundable') }}: {{ order?.order_type === 'balance' ? '$' : '¥' }}{{ maxRefundable.toFixed(2) }}
        </p>
        <p v-if="isSubscriptionOrder && form.refund_mode === 'proportional'" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('payment.admin.proportionalAmountHint') }}
        </p>
        <div v-if="isSubscriptionOrder && form.refund_mode === 'proportional'" class="mt-3 rounded-md border border-gray-200 px-3 py-3 dark:border-dark-600">
          <div class="flex items-center justify-between gap-3 text-sm">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.admin.estimatedRefundAmount') }}</span>
            <span v-if="previewLoading" class="text-gray-500 dark:text-gray-400">{{ t('common.processing') }}</span>
            <span v-else class="font-medium text-gray-900 dark:text-white">{{ order?.order_type === 'balance' ? '$' : '¥' }}{{ (previewAmount ?? 0).toFixed(2) }}</span>
          </div>
        </div>
      </div>

      <!-- Reason -->
      <div>
        <label class="input-label">{{ t('payment.admin.refundReason') }}</label>
        <textarea
          v-model="form.reason"
          rows="3"
          class="input"
          :placeholder="t('payment.admin.refundReasonPlaceholder')"
          required
        ></textarea>
      </div>

      <!-- Warning -->
      <div
        v-if="warning"
        class="rounded-lg bg-yellow-50 p-3 text-sm text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-300"
      >
        {{ warning }}
      </div>

      <!-- Force Refund -->
      <div v-if="requireForce" class="flex items-center gap-2">
        <input
          id="force-refund"
          v-model="form.force"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-red-600 focus:ring-red-500"
        />
        <label for="force-refund" class="text-sm font-medium text-red-600 dark:text-red-400">
          {{ t('payment.admin.forceRefund') }}
        </label>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="emit('cancel')" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="refund-form"
          :disabled="submitting || (!isSubscriptionOrder && form.amount <= 0) || (isSubscriptionOrder && form.refund_mode === 'full' && form.amount <= 0) || (requireForce && !form.force)"
          class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 disabled:opacity-50 dark:focus:ring-offset-dark-800"
        >
          {{ submitting ? t('common.processing') : t('payment.admin.confirmRefund') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, computed, watch, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PaymentOrder } from '@/types/payment'
import { formatOrderDateTime } from '@/components/payment/orderUtils'
import { adminPaymentAPI } from '@/api/admin/payment'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  order: PaymentOrder | null
  submitting?: boolean
  userBalance?: number | null
  requireForce?: boolean
  warning?: string
}>()

const emit = defineEmits<{
  (e: 'confirm', data: { amount: number; reason: string; deduct_balance: boolean; force: boolean; refund_mode?: 'full' | 'proportional' }): void
  (e: 'cancel'): void
}>()

const form = reactive({
  amount: 0,
  reason: '',
  deduct_balance: true,
  force: false,
  refund_mode: 'full' as 'full' | 'proportional',
})

const previewAmount = ref<number | null>(null)
const previewLoading = ref(false)

// In REFUND_REQUESTED status, refund_amount is the REQUESTED amount, not actually refunded.
// Only PARTIALLY_REFUNDED / REFUNDED have real refund amounts.
const actuallyRefunded = computed(() => {
  if (!props.order) return 0
  const s = props.order.status
  if (s === 'PARTIALLY_REFUNDED' || s === 'REFUNDED') return props.order.refund_amount || 0
  return 0
})

const maxRefundable = computed(() => {
  if (!props.order) return 0
  return props.order.amount - actuallyRefunded.value
})

const balanceInsufficient = computed(() => {
  if (props.userBalance == null || !props.order) return false
  return props.userBalance < props.order.amount
})

const isSubscriptionOrder = computed(() => props.order?.order_type === 'subscription')

watch(() => props.show, (val) => {
  if (val && props.order) {
    form.refund_mode = isSubscriptionOrder.value ? 'proportional' : 'full'
    // For REFUND_REQUESTED, pre-fill with the requested amount
    if (!isSubscriptionOrder.value && props.order.status === 'REFUND_REQUESTED' && props.order.refund_amount) {
      form.amount = props.order.refund_amount
    } else if (isSubscriptionOrder.value && form.refund_mode === 'proportional') {
      form.amount = 0
    } else {
      form.amount = maxRefundable.value
    }
    form.reason = props.order.refund_request_reason || ''
    form.deduct_balance = true
    form.force = false
    if (isSubscriptionOrder.value && form.refund_mode === 'proportional') {
      void loadPreviewAmount()
    } else {
      previewAmount.value = null
    }
  } else {
    previewAmount.value = null
  }
})

watch(() => form.refund_mode, (mode) => {
  if (!isSubscriptionOrder.value) return
  form.amount = mode === 'proportional' ? 0 : maxRefundable.value
  if (mode === 'proportional') {
    void loadPreviewAmount()
  } else {
    previewAmount.value = null
  }
})

function formatDateTime(dateStr: string): string {
  return formatOrderDateTime(dateStr)
}

async function loadPreviewAmount() {
  if (!props.order || !isSubscriptionOrder.value || form.refund_mode !== 'proportional') {
    previewAmount.value = null
    return
  }
  previewLoading.value = true
  try {
    const res = await adminPaymentAPI.previewRefund(props.order.id, { amount: 0, refund_mode: 'proportional' })
    previewAmount.value = res.data.refund_amount
  } catch {
    previewAmount.value = null
  } finally {
    previewLoading.value = false
  }
}

function handleSubmit() {
  const amount = isSubscriptionOrder.value && form.refund_mode === 'proportional' ? 0 : form.amount
  if (amount < 0 || amount > maxRefundable.value) return
  if (!isSubscriptionOrder.value && amount <= 0) return
  if (isSubscriptionOrder.value && form.refund_mode === 'full' && amount <= 0) return
  if (props.requireForce && !form.force) return
  emit('confirm', {
    amount,
    reason: form.reason,
    deduct_balance: form.deduct_balance,
    force: form.force,
    refund_mode: isSubscriptionOrder.value ? form.refund_mode : undefined,
  })
}
</script>
