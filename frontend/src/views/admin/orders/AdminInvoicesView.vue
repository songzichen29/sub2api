<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-3">
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex-1 sm:max-w-64"><input v-model="keyword" class="input w-full" :placeholder="t('invoice.admin.searchPlaceholder')" @input="debouncedLoad" /></div>
            <Select v-model="filters.status" :options="statusOptions" class="w-36" @change="loadApplications" />
            <input v-model="filters.start_date" type="date" class="input w-36" :aria-label="t('invoice.admin.startDate')" @change="loadApplications" />
            <input v-model="filters.end_date" type="date" class="input w-36" :aria-label="t('invoice.admin.endDate')" @change="loadApplications" />
            <div class="flex flex-1 justify-end gap-2">
              <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadApplications"><Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" /></button>
              <button class="btn btn-secondary" :disabled="exporting" :title="t('invoice.admin.export')" @click="exportApplications"><Icon name="download" size="md" /></button>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-3 border-t border-gray-200 pt-3 dark:border-dark-700">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ t('invoice.admin.minimumAmount') }}</span>
            <input v-model.number="minimumAmount" type="number" min="0.01" step="0.01" class="input w-32" />
            <button class="btn btn-secondary" :disabled="savingSettings" :title="t('common.save')" @click="saveSettings"><Icon name="check" size="sm" /></button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="applications" :loading="loading" row-key="id">
          <template #cell-application_no="{ value }"><span class="font-mono text-sm">{{ value }}</span></template>
          <template #cell-user="{ row }"><span class="text-sm">{{ row.user_email || row.user_name || '#' + row.user_id }}</span></template>
          <template #cell-total_amount="{ value }"><span class="font-medium">{{ formatCNY(value) }}</span></template>
          <template #cell-status="{ value }"><span :class="['badge', statusClass(value)]">{{ statusLabel(value) }}</span></template>
          <template #cell-created_at="{ value }"><span class="text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span></template>
          <template #cell-actions="{ row }"><button type="button" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="openApplication(row.id)"><Icon name="eye" size="sm" />{{ t('common.view') }}</button></template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="changePage" @update:page-size="changePageSize" />
      </template>
    </TablePageLayout>

    <BaseDialog :show="!!selectedApplication" :title="selectedApplication?.application_no || ''" width="wide" @close="selectedApplication = null">
      <div v-if="selectedApplication" class="space-y-5">
        <div class="grid gap-3 text-sm sm:grid-cols-2">
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.admin.user') }}</p><p class="mt-1 text-gray-900 dark:text-white">{{ selectedApplication.user_email || selectedApplication.user_name || '#' + selectedApplication.user_id }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.history.amount') }}</p><p class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatCNY(selectedApplication.total_amount) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.header.title') }}</p><p class="mt-1 text-gray-900 dark:text-white">{{ selectedApplication.header_title }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.header.taxNumber') }}</p><p class="mt-1 text-gray-900 dark:text-white">{{ selectedApplication.header_tax_number || '-' }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.header.email') }}</p><p class="mt-1 text-gray-900 dark:text-white">{{ selectedApplication.header_email || '-' }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoice.header.phone') }}</p><p class="mt-1 text-gray-900 dark:text-white">{{ selectedApplication.header_phone || '-' }}</p></div>
        </div>

        <div class="overflow-x-auto border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[520px] text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400"><tr><th class="px-3 py-2">{{ t('invoice.history.orderNo') }}</th><th class="px-3 py-2">{{ t('invoice.history.orderType') }}</th><th class="px-3 py-2">{{ t('invoice.history.amount') }}</th><th class="px-3 py-2">{{ t('invoice.selectOrders.paidAt') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700"><tr v-for="order in selectedApplication.orders" :key="order.order_id"><td class="px-3 py-2 font-mono">{{ order.order_no || '#' + order.order_id }}</td><td class="px-3 py-2">{{ orderTypeLabel(order.order_type) }}</td><td class="px-3 py-2">{{ formatCNY(order.amount) }}</td><td class="px-3 py-2">{{ formatDateTime(order.paid_at) }}</td></tr></tbody>
          </table>
        </div>

        <form id="invoice-process-form" class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-700" @submit.prevent="saveApplication">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('invoice.admin.processStatus') }}</label>
              <Select v-model="processForm.status" :options="processStatusOptions" class="w-full" />
            </div>
            <div v-if="processForm.status === 'INVOICED'">
              <label class="input-label">{{ t('invoice.history.invoiceNumber') }}</label>
              <input v-model.trim="processForm.invoice_number" class="input w-full" required />
            </div>
            <div v-if="processForm.status === 'REJECTED'" class="sm:col-span-2">
              <label class="input-label">{{ t('invoice.history.rejectionReason') }}</label>
              <textarea v-model.trim="processForm.rejection_reason" rows="2" class="input w-full" required />
            </div>
            <div class="sm:col-span-2">
              <label class="input-label">{{ t('invoice.admin.note') }}</label>
              <textarea v-model.trim="processForm.admin_note" rows="2" class="input w-full" />
            </div>
          </div>
        </form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="selectedApplication = null">{{ t('common.close') }}</button><button v-if="canProcess" type="submit" form="invoice-process-form" class="btn btn-primary" :disabled="savingApplication">{{ savingApplication ? t('common.saving') : t('common.save') }}</button></div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { adminInvoiceAPI, type UpdateInvoiceApplicationInput } from '@/api/admin/invoices'
import type { InvoiceApplication, InvoiceApplicationStatus } from '@/api/invoice'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const applications = ref<InvoiceApplication[]>([])
const selectedApplication = ref<InvoiceApplication | null>(null)
const loading = ref(false)
const savingSettings = ref(false)
const savingApplication = ref(false)
const exporting = ref(false)
const keyword = ref('')
const minimumAmount = ref(300)
const filters = reactive<{ status: '' | InvoiceApplicationStatus; start_date: string; end_date: string }>({ status: '', start_date: '', end_date: '' })
const pagination = reactive({ page: 1, page_size: getPersistedPageSize(), total: 0 })
const processForm = reactive<UpdateInvoiceApplicationInput>({ status: 'PROCESSING', rejection_reason: '', admin_note: '', invoice_number: '' })
let searchTimer: ReturnType<typeof setTimeout> | null = null

const columns = computed<Column[]>(() => [
  { key: 'application_no', label: t('invoice.history.applicationNo') }, { key: 'user', label: t('invoice.admin.user') },
  { key: 'header_title', label: t('invoice.header.title') }, { key: 'total_amount', label: t('invoice.history.amount') },
  { key: 'status', label: t('invoice.history.status') }, { key: 'created_at', label: t('invoice.history.createdAt') }, { key: 'actions', label: t('common.actions') },
])
const statusOptions = computed(() => [{ value: '', label: t('invoice.admin.allStatuses') }, ...(['PENDING', 'PROCESSING', 'INVOICED', 'REJECTED'] as InvoiceApplicationStatus[]).map(value => ({ value, label: statusLabel(value) }))])
const processStatusOptions = computed(() => {
  if (!selectedApplication.value) return []
  const allowed: InvoiceApplicationStatus[] = selectedApplication.value.status === 'PENDING' ? ['PROCESSING', 'INVOICED', 'REJECTED'] : selectedApplication.value.status === 'PROCESSING' ? ['PROCESSING', 'INVOICED', 'REJECTED'] : []
  return allowed.map(value => ({ value, label: statusLabel(value) }))
})
const canProcess = computed(() => selectedApplication.value?.status === 'PENDING' || selectedApplication.value?.status === 'PROCESSING')

function formatCNY(value: number) { return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'CNY', minimumFractionDigits: 2 }).format(Number(value || 0)) }
function statusLabel(status: string) { return t(`invoice.status.${status}`, status) }
function statusClass(status: string) { return ({ PENDING: 'badge-warning', PROCESSING: 'badge-info', INVOICED: 'badge-success', REJECTED: 'badge-danger' } as Record<string, string>)[status] || 'badge-gray' }
function orderTypeLabel(type: string) { return t(`invoice.orderTypes.${type}`, type) }
function requestParams() { return { page: pagination.page, page_size: pagination.page_size, status: filters.status || undefined, keyword: keyword.value || undefined, start_date: filters.start_date || undefined, end_date: filters.end_date || undefined } }

async function loadApplications() {
  loading.value = true
  try {
    const response = await adminInvoiceAPI.listApplications(requestParams())
    applications.value = response.data.items || []
    pagination.total = response.data.total || 0
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) } finally { loading.value = false }
}
async function loadSettings() {
  try { const response = await adminInvoiceAPI.getSettings(); minimumAmount.value = Number(response.data.min_amount || 300) } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) }
}
async function saveSettings() {
  savingSettings.value = true
  try { const response = await adminInvoiceAPI.updateSettings(minimumAmount.value); minimumAmount.value = response.data.min_amount; appStore.showSuccess(t('common.success')) } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) } finally { savingSettings.value = false }
}
async function openApplication(id: number) {
  try {
    const response = await adminInvoiceAPI.getApplication(id)
    selectedApplication.value = response.data
    processForm.status = response.data.status === 'PENDING' ? 'PROCESSING' : response.data.status === 'PROCESSING' ? 'PROCESSING' : 'INVOICED'
    processForm.rejection_reason = response.data.rejection_reason || ''
    processForm.admin_note = response.data.admin_note || ''
    processForm.invoice_number = response.data.invoice_number || ''
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) }
}
async function saveApplication() {
  if (!selectedApplication.value || !canProcess.value) return
  savingApplication.value = true
  try {
    const response = await adminInvoiceAPI.updateApplication(selectedApplication.value.id, { ...processForm })
    selectedApplication.value = response.data
    appStore.showSuccess(t('common.success'))
    await loadApplications()
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) } finally { savingApplication.value = false }
}
async function exportApplications() {
  exporting.value = true
  try {
    const response = await adminInvoiceAPI.exportApplications({ status: filters.status || undefined, keyword: keyword.value || undefined, start_date: filters.start_date || undefined, end_date: filters.end_date || undefined })
    const url = URL.createObjectURL(response.data as Blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'invoice-applications.csv'
    anchor.click()
    URL.revokeObjectURL(url)
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) } finally { exporting.value = false }
}
function debouncedLoad() { if (searchTimer) clearTimeout(searchTimer); searchTimer = setTimeout(() => { pagination.page = 1; loadApplications() }, 300) }
function changePage(page: number) { pagination.page = page; loadApplications() }
function changePageSize(size: number) { pagination.page_size = size; pagination.page = 1; loadApplications() }
onMounted(() => { loadSettings(); loadApplications() })
</script>
