<template>
  <DataTable :columns="columns" :data="orders" :loading="loading">
    <template #cell-id="{ value }">
      <span class="font-mono text-sm">#{{ value }}</span>
    </template>
    <template #cell-out_trade_no="{ value }">
      <span class="text-sm text-gray-900 dark:text-white">{{ value }}</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row }">
      <div class="text-sm">
        <span class="text-gray-900 dark:text-white">{{ value || row.user_name || '#' + row.user_id }}</span>
        <span v-if="row.user_notes" class="ml-1 text-xs text-gray-400">({{ row.user_notes }})</span>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row }">
      <div class="text-sm">
        <span class="font-medium text-gray-900 dark:text-white">¥{{ value.toFixed(2) }}</span>
        <span v-if="row.fee_rate > 0" class="ml-1 text-xs text-gray-400" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)
        </span>
        <div v-if="row.amount !== row.pay_amount" class="text-xs text-gray-500">
          {{ t('payment.orders.creditedAmount') }}: {{ row.order_type === 'balance' ? '$' : '¥' }}{{ row.amount.toFixed(2) }}
        </div>
      </div>
    </template>
    <template #cell-payment_type="{ value }">
      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) }}</span>
    </template>
    <template #cell-order_type="{ value, row }">
      <div class="text-sm">
        <div class="text-gray-900 dark:text-white">{{ orderTypeLabel(value) }}</div>
        <div v-if="orderContentLabel(row)" class="text-xs text-gray-500 dark:text-gray-400">
          {{ orderContentLabel(row) }}
        </div>
      </div>
    </template>
    <template #cell-status="{ value }">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value }">
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <slot name="actions" :row="row" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'

const { t } = useI18n()

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

function orderTypeLabel(orderType: PaymentOrder['order_type']): string {
  if (orderType === 'subscription') return t('payment.admin.subscriptionOrder')
  if (orderType === 'daily_limit_reset') return t('payment.admin.dailyLimitResetOrder')
  return t('payment.admin.balanceOrder')
}

function orderContentLabel(row: PaymentOrder): string {
  if (row.order_type === 'subscription') {
    const name = row.product_name || row.group_name
    const fixedExpiresAt = formatFixedSubscriptionExpiresAt(row.subscription_plan_expires_at)
    if (name && fixedExpiresAt) return `${name} · ${t('payment.admin.fixedExpiresAtShort', { time: fixedExpiresAt })}`
    if (fixedExpiresAt) return t('payment.admin.fixedExpiresAtShort', { time: fixedExpiresAt })
    if (name && row.subscription_days) return `${name} · ${row.subscription_days}${t('payment.admin.days')}`
    if (name) return name
    if (row.subscription_days) return `${row.subscription_days}${t('payment.admin.days')}`
    return ''
  }
  if (row.order_type === 'daily_limit_reset') {
    return row.group_name || t('payment.admin.dailyLimitResetOrder')
  }
  return ''
}

function formatFixedSubscriptionExpiresAt(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

const columns = computed((): Column[] => {
  const cols: Column[] = [
    { key: 'id', label: t('payment.orders.orderId') },
    { key: 'out_trade_no', label: t('payment.orders.orderNo') },
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') })
  }
  cols.push(
    { key: 'order_type', label: t('payment.orders.orderType') },
    { key: 'pay_amount', label: t('payment.orders.payAmount') },
    { key: 'payment_type', label: t('payment.orders.paymentMethod') },
    { key: 'status', label: t('payment.orders.status') },
    { key: 'created_at', label: t('payment.orders.createdAt') },
    { key: 'actions', label: t('common.actions') },
  )
  return cols
})
</script>
