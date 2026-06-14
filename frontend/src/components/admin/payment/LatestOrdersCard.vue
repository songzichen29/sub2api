<template>
  <div class="card p-5">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ t('payment.admin.latestOrders') }}
      </h3>
      <RouterLink
        to="/admin/orders"
        class="inline-flex items-center rounded-md border border-primary-200 px-3 py-1.5 text-xs font-medium text-primary-700 transition-colors hover:bg-primary-50 dark:border-primary-800 dark:text-primary-300 dark:hover:bg-primary-900/20"
      >
        {{ t('common.view') }}
      </RouterLink>
    </div>

    <div v-if="loading" class="flex h-40 items-center justify-center">
      <LoadingSpinner size="md" />
    </div>

    <div v-else-if="!orders.length" class="flex h-40 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('payment.admin.noData') }}
    </div>

    <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-400">
          <tr>
            <th class="px-4 py-3 text-left">{{ t('payment.orders.orderNo') }}</th>
            <th class="px-4 py-3 text-left">{{ t('payment.orders.userId') }}</th>
            <th class="px-4 py-3 text-left">{{ t('payment.orders.paymentMethod') }}</th>
            <th class="px-4 py-3 text-right">{{ t('payment.orders.payAmount') }}</th>
            <th class="px-4 py-3 text-center">{{ t('payment.orders.status') }}</th>
            <th class="px-4 py-3 text-right">{{ t('payment.orders.createdAt') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="order in orders" :key="order.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-800/50">
            <td class="max-w-[150px] truncate px-4 py-3 font-mono text-xs text-gray-600 dark:text-gray-300" :title="order.out_trade_no">
              {{ shortOrderNo(order) }}
            </td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ order.user_id }}</td>
            <td class="px-4 py-3">
              <div class="flex items-center gap-2 text-gray-700 dark:text-gray-300">
                <span class="flex h-6 w-6 items-center justify-center rounded-full text-white" :style="{ backgroundColor: methodColor(order.payment_type) }">
                  <Icon :name="methodIcon(order.payment_type)" size="xs" :stroke-width="2" />
                </span>
                <span class="truncate">{{ t('payment.methods.' + order.payment_type, order.payment_type) }}</span>
              </div>
            </td>
            <td class="px-4 py-3 text-right font-medium text-gray-900 dark:text-white">${{ formatMoney(order.pay_amount) }}</td>
            <td class="px-4 py-3 text-center">
              <OrderStatusBadge :status="order.status" />
            </td>
            <td class="px-4 py-3 text-right text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(order.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { formatOrderDateTime } from '@/components/payment/orderUtils'

defineProps<{
  orders: PaymentOrder[]
  loading?: boolean
}>()

const { t } = useI18n()

function shortOrderNo(order: PaymentOrder): string {
  return order.out_trade_no || String(order.id)
}

function formatMoney(value: number): string {
  return value.toFixed(2)
}

function formatDateTime(value: string): string {
  return formatOrderDateTime(value)
}

function methodColor(type: string): string {
  const colors: Record<string, string> = {
    alipay: '#3b82f6',
    alipay_direct: '#60a5fa',
    wxpay: '#10b981',
    wxpay_direct: '#34d399',
    stripe: '#a855f7',
    airwallex: '#f97316',
    easypay: '#f59e0b',
  }
  return colors[type] || '#64748b'
}

function methodIcon(type: string): 'creditCard' | 'dollar' {
  if (type === 'wxpay' || type === 'wxpay_direct') return 'dollar'
  return 'creditCard'
}
</script>
