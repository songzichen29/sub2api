<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input
              v-model="searchQuery"
              type="text"
              class="input"
              :placeholder="t('payment.admin.coupons.search')"
              @input="handleSearch"
            />
          </div>
          <Select v-model="filters.status" :options="statusFilterOptions" class="w-36" @change="loadCoupons" />
          <div class="flex flex-1 items-center justify-end gap-2">
            <button @click="loadCoupons" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('payment.admin.coupons.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="coupons" :loading="loading">
          <template #cell-code="{ value }">
            <code class="font-mono text-sm text-gray-900 dark:text-gray-100">{{ value }}</code>
          </template>
          <template #cell-type="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ couponTypeLabel(row) }}</span>
          </template>
          <template #cell-min_amount="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">${{ Number(value || 0).toFixed(2) }}</span>
          </template>
          <template #cell-scope="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ scopeLabel(value) }}</span>
          </template>
          <template #cell-usage="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ row.used_count }} / {{ row.max_uses === 0 ? '∞' : row.max_uses }}
            </span>
          </template>
          <template #cell-status="{ row }">
            <span :class="['badge', statusClass(row)]">{{ statusLabel(row) }}</span>
          </template>
          <template #cell-expires_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ value ? formatDateTime(value) : t('payment.admin.coupons.neverExpires') }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-600"
                @click="openUsagesDialog(row)"
              >
                <Icon name="eye" size="sm" />
                {{ t('common.view') }}
              </button>
              <button
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-600"
                @click="openEditDialog(row)"
              >
                <Icon name="edit" size="sm" />
                {{ t('common.edit') }}
              </button>
              <button
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                @click="confirmDelete(row)"
              >
                <Icon name="trash" size="sm" />
                {{ t('common.delete') }}
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showEditor" :title="editorTitle" width="wide" @close="closeEditor">
      <form id="coupon-form" class="space-y-4" @submit.prevent="saveCoupon">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">
              {{ t('payment.admin.coupons.code') }}
              <span v-if="!editingCoupon" class="ml-1 text-xs font-normal text-gray-400">
                ({{ t('payment.admin.coupons.autoGenerate') }})
              </span>
            </label>
            <input v-model="form.code" type="text" class="input font-mono uppercase" :placeholder="t('payment.admin.coupons.codePlaceholder')" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.status') }}</label>
            <Select v-model="form.status" :options="editableStatusOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.type') }}</label>
            <Select v-model="form.type" :options="typeOptions" />
          </div>
          <div>
            <label class="input-label">{{ form.type === 'percent' ? t('payment.admin.coupons.percentValue') : t('payment.admin.coupons.fixedValue') }}</label>
            <input v-model.number="form.value" type="number" class="input" :step="form.type === 'percent' ? '0.01' : '0.01'" min="0" required />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.minAmount') }}</label>
            <input v-model.number="form.min_amount" type="number" class="input" step="0.01" min="0" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.maxDiscount') }}</label>
            <input v-model.number="form.max_discount" type="number" class="input" step="0.01" min="0" :disabled="form.type !== 'percent'" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.scope') }}</label>
            <Select v-model="form.scope" :options="scopeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.maxUses') }}</label>
            <input v-model.number="form.max_uses" type="number" class="input" min="0" />
            <p class="mt-1 text-xs text-gray-400">{{ t('payment.admin.coupons.zeroUnlimited') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.perUserLimit') }}</label>
            <input v-model.number="form.per_user_limit" type="number" class="input" min="0" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.startsAt') }}</label>
            <input v-model="form.starts_at_str" type="datetime-local" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.coupons.expiresAt') }}</label>
            <input v-model="form.expires_at_str" type="datetime-local" class="input" />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">{{ t('payment.admin.coupons.notes') }}</label>
            <textarea v-model="form.notes" rows="2" class="input" :placeholder="t('payment.admin.coupons.notesPlaceholder')" />
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeEditor">{{ t('common.cancel') }}</button>
          <button type="submit" form="coupon-form" class="btn btn-primary" :disabled="saving">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="showUsages" :title="t('payment.admin.coupons.usageRecords')" width="wide" @close="showUsages = false">
      <DataTable :columns="usageColumns" :data="usages" :loading="usagesLoading">
        <template #cell-user_id="{ value }">
          <span class="font-mono text-sm">#{{ value }}</span>
        </template>
        <template #cell-order_id="{ value }">
          <span class="font-mono text-sm">#{{ value }}</span>
        </template>
        <template #cell-discount_amount="{ value }">
          <span class="text-sm font-medium text-gray-900 dark:text-white">${{ Number(value || 0).toFixed(2) }}</span>
        </template>
        <template #cell-used_at="{ value }">
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span>
        </template>
        <template #cell-status="{ value }">
          <span :class="['badge', value === 'refunded' ? 'badge-gray' : 'badge-success']">{{ usageStatusLabel(value) }}</span>
        </template>
      </DataTable>
      <div v-if="usagesPagination.total > usagesPagination.page_size" class="mt-4">
        <Pagination
          :page="usagesPagination.page"
          :total="usagesPagination.total"
          :page-size="usagesPagination.page_size"
          @update:page="handleUsagePageChange"
          @update:page-size="handleUsagePageSizeChange"
        />
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary" @click="showUsages = false">{{ t('common.close') }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDelete"
      :title="t('payment.admin.coupons.deleteTitle')"
      :message="t('payment.admin.coupons.deleteConfirm')"
      :confirm-text="t('common.delete')"
      danger
      @confirm="deleteCoupon"
      @cancel="showDelete = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import type { Column } from '@/components/common/types'
import type {
  CreatePaymentCouponRequest,
  PaymentCoupon,
  PaymentCouponScope,
  PaymentCouponStatus,
  PaymentCouponType,
  PaymentCouponUsage,
  UpdatePaymentCouponRequest
} from '@/api/admin/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const coupons = ref<PaymentCoupon[]>([])
const usages = ref<PaymentCouponUsage[]>([])
const loading = ref(false)
const usagesLoading = ref(false)
const saving = ref(false)
const searchQuery = ref('')
const editingCoupon = ref<PaymentCoupon | null>(null)
const deletingCoupon = ref<PaymentCoupon | null>(null)
const viewingCoupon = ref<PaymentCoupon | null>(null)
const showEditor = ref(false)
const showDelete = ref(false)
const showUsages = ref(false)

const filters = reactive({ status: '' })
const pagination = reactive({ page: 1, page_size: getPersistedPageSize(), total: 0 })
const usagesPagination = reactive({ page: 1, page_size: 20, total: 0 })

const form = reactive({
  code: '',
  type: 'fixed' as PaymentCouponType,
  value: 1,
  min_amount: 0,
  max_discount: 0,
  scope: 'all' as PaymentCouponScope,
  max_uses: 0,
  per_user_limit: 1,
  status: 'active' as PaymentCouponStatus,
  starts_at_str: '',
  expires_at_str: '',
  notes: ''
})

const columns = computed<Column[]>(() => [
  { key: 'code', label: t('payment.admin.coupons.columns.code') },
  { key: 'type', label: t('payment.admin.coupons.columns.type') },
  { key: 'min_amount', label: t('payment.admin.coupons.columns.minAmount') },
  { key: 'scope', label: t('payment.admin.coupons.columns.scope') },
  { key: 'usage', label: t('payment.admin.coupons.columns.usage') },
  { key: 'status', label: t('payment.admin.coupons.columns.status') },
  { key: 'expires_at', label: t('payment.admin.coupons.columns.expiresAt') },
  { key: 'actions', label: t('common.actions') }
])

const usageColumns = computed<Column[]>(() => [
  { key: 'user_id', label: t('payment.admin.coupons.columns.userId') },
  { key: 'order_id', label: t('payment.admin.coupons.columns.orderId') },
  { key: 'discount_amount', label: t('payment.admin.coupons.columns.discountAmount') },
  { key: 'used_at', label: t('payment.admin.coupons.columns.usedAt') },
  { key: 'status', label: t('payment.admin.coupons.columns.status') }
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.coupons.allStatuses') },
  { value: 'active', label: t('payment.admin.coupons.statusActive') },
  { value: 'disabled', label: t('payment.admin.coupons.statusDisabled') },
  { value: 'archived', label: t('payment.admin.coupons.statusArchived') }
])

const editableStatusOptions = computed(() => [
  { value: 'active', label: t('payment.admin.coupons.statusActive') },
  { value: 'disabled', label: t('payment.admin.coupons.statusDisabled') },
  { value: 'archived', label: t('payment.admin.coupons.statusArchived') }
])

const typeOptions = computed(() => [
  { value: 'fixed', label: t('payment.admin.coupons.typeFixed') },
  { value: 'percent', label: t('payment.admin.coupons.typePercent') }
])

const scopeOptions = computed(() => [
  { value: 'all', label: t('payment.admin.coupons.scopeAll') },
  { value: 'balance', label: t('payment.admin.coupons.scopeBalance') },
  { value: 'subscription', label: t('payment.admin.coupons.scopeSubscription') }
])

const editorTitle = computed(() => editingCoupon.value ? t('payment.admin.coupons.edit') : t('payment.admin.coupons.create'))

let searchTimer: ReturnType<typeof setTimeout> | null = null

function handleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.page = 1
    loadCoupons()
  }, 300)
}

async function loadCoupons() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.listCoupons({
      page: pagination.page,
      page_size: pagination.page_size,
      status: filters.status || undefined,
      search: searchQuery.value || undefined
    })
    coupons.value = res.data.items || []
    pagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('payment.admin.coupons.failedToLoad')))
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  loadCoupons()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  loadCoupons()
}

function resetForm() {
  Object.assign(form, {
    code: '',
    type: 'fixed',
    value: 1,
    min_amount: 0,
    max_discount: 0,
    scope: 'all',
    max_uses: 0,
    per_user_limit: 1,
    status: 'active',
    starts_at_str: '',
    expires_at_str: '',
    notes: ''
  })
}

function openCreateDialog() {
  editingCoupon.value = null
  resetForm()
  showEditor.value = true
}

function openEditDialog(coupon: PaymentCoupon) {
  editingCoupon.value = coupon
  Object.assign(form, {
    code: coupon.code,
    type: coupon.type,
    value: coupon.value,
    min_amount: coupon.min_amount,
    max_discount: coupon.max_discount,
    scope: coupon.scope,
    max_uses: coupon.max_uses,
    per_user_limit: coupon.per_user_limit,
    status: coupon.status,
    starts_at_str: toDateTimeLocal(coupon.starts_at),
    expires_at_str: toDateTimeLocal(coupon.expires_at),
    notes: coupon.notes || ''
  })
  showEditor.value = true
}

function closeEditor() {
  showEditor.value = false
  editingCoupon.value = null
}

async function saveCoupon() {
  saving.value = true
  try {
    const payload: CreatePaymentCouponRequest | UpdatePaymentCouponRequest = {
      code: form.code.trim() || undefined,
      type: form.type,
      value: Number(form.value) || 0,
      min_amount: Number(form.min_amount) || 0,
      max_discount: form.type === 'percent' ? Number(form.max_discount) || 0 : 0,
      scope: form.scope,
      max_uses: Number(form.max_uses) || 0,
      per_user_limit: Number(form.per_user_limit) || 0,
      starts_at: toUnix(form.starts_at_str),
      expires_at: toUnix(form.expires_at_str),
      notes: form.notes || undefined
    }
    if (editingCoupon.value) {
      await adminPaymentAPI.updateCoupon(editingCoupon.value.id, { ...payload, status: form.status })
      appStore.showSuccess(t('payment.admin.coupons.updated'))
    } else {
      await adminPaymentAPI.createCoupon(payload as CreatePaymentCouponRequest)
      appStore.showSuccess(t('payment.admin.coupons.created'))
    }
    closeEditor()
    loadCoupons()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('payment.admin.coupons.failedToSave')))
  } finally {
    saving.value = false
  }
}

function confirmDelete(coupon: PaymentCoupon) {
  deletingCoupon.value = coupon
  showDelete.value = true
}

async function deleteCoupon() {
  if (!deletingCoupon.value) return
  try {
    await adminPaymentAPI.deleteCoupon(deletingCoupon.value.id)
    appStore.showSuccess(t('payment.admin.coupons.deleted'))
    showDelete.value = false
    deletingCoupon.value = null
    loadCoupons()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('payment.admin.coupons.failedToDelete')))
  }
}

async function openUsagesDialog(coupon: PaymentCoupon) {
  viewingCoupon.value = coupon
  usagesPagination.page = 1
  showUsages.value = true
  await loadUsages()
}

async function loadUsages() {
  if (!viewingCoupon.value) return
  usagesLoading.value = true
  try {
    const res = await adminPaymentAPI.listCouponUsages(viewingCoupon.value.id, {
      page: usagesPagination.page,
      page_size: usagesPagination.page_size
    })
    usages.value = res.data.items || []
    usagesPagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('payment.admin.coupons.failedToLoadUsages')))
  } finally {
    usagesLoading.value = false
  }
}

function handleUsagePageChange(page: number) {
  usagesPagination.page = page
  loadUsages()
}

function handleUsagePageSizeChange(size: number) {
  usagesPagination.page_size = size
  usagesPagination.page = 1
  loadUsages()
}

function couponTypeLabel(coupon: PaymentCoupon): string {
  if (coupon.type === 'percent') return `${Math.round(coupon.value * 100)}%`
  return `$${coupon.value.toFixed(2)}`
}

function scopeLabel(scope: string): string {
  const option = scopeOptions.value.find(item => item.value === scope)
  return option?.label || scope
}

function statusClass(coupon: PaymentCoupon): string {
  if (coupon.status === 'archived') return 'badge-gray'
  if (coupon.status === 'disabled') return 'badge-gray'
  if (coupon.expires_at && new Date(coupon.expires_at).getTime() < Date.now()) return 'badge-danger'
  if (coupon.max_uses > 0 && coupon.used_count >= coupon.max_uses) return 'badge-gray'
  return 'badge-success'
}

function statusLabel(coupon: PaymentCoupon): string {
  if (coupon.status === 'archived') return t('payment.admin.coupons.statusArchived')
  if (coupon.status === 'disabled') return t('payment.admin.coupons.statusDisabled')
  if (coupon.expires_at && new Date(coupon.expires_at).getTime() < Date.now()) return t('payment.admin.coupons.statusExpired')
  if (coupon.max_uses > 0 && coupon.used_count >= coupon.max_uses) return t('payment.admin.coupons.statusMaxUsed')
  return t('payment.admin.coupons.statusActive')
}

function usageStatusLabel(status: string): string {
  return status === 'refunded' ? t('payment.admin.coupons.usageRefunded') : t('payment.admin.coupons.usageUsed')
}

function toUnix(value: string): number | undefined {
  if (!value) return undefined
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? undefined : Math.floor(time / 1000)
}

function toDateTimeLocal(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toISOString().slice(0, 16)
}

onMounted(() => loadCoupons())
</script>
