<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="flex items-center gap-1" role="tablist">
          <button
            type="button"
            class="px-3 py-2 text-sm font-medium"
            :class="activeTab === 'apply' ? 'border-b-2 border-primary-600 text-primary-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'"
            @click="activeTab = 'apply'"
          >
            {{ t('invoice.tabs.apply') }}
          </button>
          <button
            type="button"
            class="px-3 py-2 text-sm font-medium"
            :class="activeTab === 'history' ? 'border-b-2 border-primary-600 text-primary-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'"
            @click="activeTab = 'history'"
          >
            {{ t('invoice.tabs.history') }}
          </button>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-secondary" @click="showHeaderManager = true"><Icon name="document" size="sm" class="mr-1" />{{ t('invoice.header.manage') }}</button>
          <button class="btn btn-primary" @click="openHeaderEditor()"><Icon name="plus" size="sm" class="mr-1" />{{ t('invoice.header.add') }}</button>
        </div>
      </div>

      <template v-if="activeTab === 'apply'">
        <section v-if="step === 'orders'" class="space-y-4">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('invoice.selectOrders.title') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.selectOrders.minimum', { amount: formatCNY(minAmount) }) }}</p>
            </div>
            <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadData">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>

          <DataTable :columns="orderColumns" :data="eligibleOrders" :loading="loading" row-key="id">
            <template #cell-select="{ row }">
              <input
                :checked="isSelected(row.id)"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                :aria-label="t('invoice.selectOrders.selectOrder', { id: row.id })"
                @change="toggleOrder(row.id)"
              />
            </template>
            <template #cell-id="{ value }"><span class="font-mono text-sm">#{{ value }}</span></template>
            <template #cell-order_no="{ value }"><span class="font-mono text-sm text-gray-900 dark:text-white">{{ value || '-' }}</span></template>
            <template #cell-order_type="{ value }"><span class="text-sm">{{ orderTypeLabel(value) }}</span></template>
            <template #cell-amount="{ value }"><span class="font-medium text-gray-900 dark:text-white">{{ formatCNY(value) }}</span></template>
            <template #cell-paid_at="{ value }"><span class="text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span></template>
            <template #empty>
              <div class="py-4 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.selectOrders.empty') }}</div>
            </template>
          </DataTable>

          <div class="sticky bottom-3 z-10 flex flex-col gap-3 border border-gray-200 bg-white px-4 py-3 shadow-lg dark:border-dark-700 dark:bg-dark-900 sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('invoice.summary.selected', { count: selectedOrders.length, amount: formatCNY(selectedTotal) }) }}</p>
              <p :class="['mt-1 text-xs', meetsMinimum ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300']">
                {{ meetsMinimum ? t('invoice.summary.met') : t('invoice.summary.remaining', { amount: formatCNY(remainingAmount) }) }}
              </p>
            </div>
            <button class="btn btn-primary" :disabled="!meetsMinimum" @click="step = 'confirm'">
              {{ t('common.next') }}
              <Icon name="arrowRight" size="sm" class="ml-1" />
            </button>
          </div>
        </section>

        <section v-else class="mx-auto max-w-3xl space-y-5">
          <div class="flex items-center justify-between border-b border-gray-200 pb-3 dark:border-dark-700">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('invoice.confirm.title') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.confirm.total', { amount: formatCNY(selectedTotal), count: selectedOrders.length }) }}</p>
            </div>
            <button type="button" class="btn btn-secondary" @click="step = 'orders'">
              <Icon name="arrowLeft" size="sm" class="mr-1" />{{ t('common.previous') }}
            </button>
          </div>

          <div class="space-y-2">
            <div class="flex items-center justify-between gap-3">
              <label class="input-label">{{ t('invoice.header.choose') }}</label>
              <div class="flex gap-3"><button type="button" class="text-sm text-primary-700 hover:underline dark:text-primary-300" @click="showHeaderManager = true">{{ t('invoice.header.manage') }}</button><button type="button" class="text-sm text-primary-700 hover:underline dark:text-primary-300" @click="openHeaderEditor()">{{ t('invoice.header.add') }}</button></div>
            </div>
            <Select v-model="selectedHeaderID" :options="headerOptions" class="w-full" />
            <div v-if="selectedHeader" class="border-l-2 border-primary-500 bg-gray-50 px-3 py-3 text-sm dark:bg-dark-800">
              <div class="font-medium text-gray-900 dark:text-white">{{ selectedHeader.title }}</div>
              <div v-if="selectedHeader.tax_number" class="mt-1 text-gray-600 dark:text-gray-300">{{ t('invoice.header.taxNumber') }}: {{ selectedHeader.tax_number }}</div>
              <div v-if="selectedHeader.email" class="mt-1 text-gray-600 dark:text-gray-300">{{ selectedHeader.email }}</div>
            </div>
            <p v-else class="text-sm text-amber-700 dark:text-amber-300">{{ t('invoice.header.required') }}</p>
          </div>

          <div class="border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-100">
            {{ t('invoice.confirm.warning') }}
          </div>
          <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-gray-200">
            <input v-model="acknowledged" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            <span>{{ t('invoice.confirm.acknowledge') }}</span>
          </label>
          <div class="flex justify-end gap-3 border-t border-gray-200 pt-4 dark:border-dark-700">
            <button type="button" class="btn btn-secondary" @click="step = 'orders'">{{ t('common.previous') }}</button>
            <button type="button" class="btn btn-primary" :disabled="submitting || !selectedHeaderID || !acknowledged" @click="submitApplication">
              {{ submitting ? t('common.submitting') : t('invoice.confirm.submit') }}
            </button>
          </div>
        </section>
      </template>

      <section v-else class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('invoice.history.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.history.subtitle') }}</p>
          </div>
          <button class="btn btn-secondary" :disabled="historyLoading" :title="t('common.refresh')" @click="loadApplications">
            <Icon name="refresh" size="md" :class="historyLoading ? 'animate-spin' : ''" />
          </button>
        </div>
        <DataTable :columns="applicationColumns" :data="applications" :loading="historyLoading" row-key="id">
          <template #cell-application_no="{ value }"><span class="font-mono text-sm">{{ value }}</span></template>
          <template #cell-total_amount="{ value }"><span class="font-medium">{{ formatCNY(value) }}</span></template>
          <template #cell-status="{ value }"><span :class="['badge', statusClass(value)]">{{ statusLabel(value) }}</span></template>
          <template #cell-created_at="{ value }"><span class="text-sm text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span></template>
          <template #cell-actions="{ row }">
            <button type="button" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="openApplication(row)">
              <Icon name="eye" size="sm" />{{ t('common.view') }}
            </button>
          </template>
        </DataTable>
        <Pagination
          v-if="historyPagination.total > 0"
          :page="historyPagination.page"
          :total="historyPagination.total"
          :page-size="historyPagination.page_size"
          @update:page="changeHistoryPage"
          @update:page-size="changeHistoryPageSize"
        />
      </section>
    </div>

    <BaseDialog :show="showHeaderEditor" :title="editingHeader ? t('invoice.header.edit') : t('invoice.header.add')" width="wide" @close="closeHeaderEditor">
      <form id="invoice-header-form" class="space-y-4" @submit.prevent="saveHeader">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('invoice.header.type') }}</label>
            <Select v-model="headerForm.title_type" :options="headerTypeOptions" class="w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('invoice.header.title') }}</label>
            <input v-model.trim="headerForm.title" class="input w-full" required />
          </div>
          <div v-if="headerForm.title_type === 'company'">
            <label class="input-label">{{ t('invoice.header.taxNumber') }}</label>
            <input v-model.trim="headerForm.tax_number" class="input w-full" required />
          </div>
          <div>
            <label class="input-label">{{ t('invoice.header.email') }}</label>
            <input v-model.trim="headerForm.email" type="email" class="input w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('invoice.header.phone') }}</label>
            <input v-model.trim="headerForm.phone" type="tel" class="input w-full" />
          </div>
          <div class="md:col-span-2">
            <label class="input-label">{{ t('invoice.header.address') }}</label>
            <textarea v-model.trim="headerForm.address" rows="2" class="input w-full" />
          </div>
        </div>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="headerForm.is_default" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('invoice.header.default') }}
        </label>
      </form>
      <template #footer>
        <div class="flex justify-between gap-3">
          <button v-if="editingHeader" type="button" class="btn btn-danger" :disabled="headerSaving" @click="deleteHeader(editingHeader)">
            <Icon name="trash" size="sm" class="mr-1" />{{ t('common.delete') }}
          </button>
          <span v-else></span>
          <div class="flex gap-3">
            <button type="button" class="btn btn-secondary" @click="closeHeaderEditor">{{ t('common.cancel') }}</button>
            <button type="submit" form="invoice-header-form" class="btn btn-primary" :disabled="headerSaving">{{ headerSaving ? t('common.saving') : t('common.save') }}</button>
          </div>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="!!viewingApplication" :title="viewingApplication?.application_no || ''" width="wide" @close="viewingApplication = null">
      <div v-if="viewingApplication" class="space-y-4 text-sm">
        <div class="grid gap-3 sm:grid-cols-2">
          <div><span class="text-gray-500 dark:text-gray-400">{{ t('invoice.history.status') }}</span><span :class="['ml-2 badge', statusClass(viewingApplication.status)]">{{ statusLabel(viewingApplication.status) }}</span></div>
          <div><span class="text-gray-500 dark:text-gray-400">{{ t('invoice.history.amount') }}</span><span class="ml-2 font-medium">{{ formatCNY(viewingApplication.total_amount) }}</span></div>
          <div><span class="text-gray-500 dark:text-gray-400">{{ t('invoice.header.title') }}</span><span class="ml-2">{{ viewingApplication.header_title }}</span></div>
          <div v-if="viewingApplication.header_tax_number"><span class="text-gray-500 dark:text-gray-400">{{ t('invoice.header.taxNumber') }}</span><span class="ml-2">{{ viewingApplication.header_tax_number }}</span></div>
        </div>
        <div v-if="viewingApplication.rejection_reason" class="border-l-2 border-red-500 bg-red-50 px-3 py-2 text-red-800 dark:bg-red-950/30 dark:text-red-200">
          {{ t('invoice.history.rejectionReason') }}: {{ viewingApplication.rejection_reason }}
        </div>
        <div v-if="viewingApplication.invoice_number" class="border-l-2 border-emerald-500 bg-emerald-50 px-3 py-2 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-200">
          {{ t('invoice.history.invoiceNumber') }}: {{ viewingApplication.invoice_number }}
        </div>
        <div class="overflow-x-auto border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[480px] text-left">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400"><tr><th class="px-3 py-2">{{ t('invoice.history.orderNo') }}</th><th class="px-3 py-2">{{ t('invoice.history.orderType') }}</th><th class="px-3 py-2">{{ t('invoice.history.amount') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700"><tr v-for="order in viewingApplication.orders" :key="order.order_id"><td class="px-3 py-2 font-mono">{{ order.order_no || '#' + order.order_id }}</td><td class="px-3 py-2">{{ orderTypeLabel(order.order_type) }}</td><td class="px-3 py-2">{{ formatCNY(order.amount) }}</td></tr></tbody>
          </table>
        </div>
      </div>
      <template #footer><button type="button" class="btn btn-secondary" @click="viewingApplication = null">{{ t('common.close') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="showHeaderManager" :title="t('invoice.header.manage')" width="wide" @close="showHeaderManager = false">
      <div class="divide-y divide-gray-200 border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <div v-for="header in headers" :key="header.id" class="flex items-center justify-between gap-3 px-4 py-3">
          <div class="min-w-0"><div class="flex items-center gap-2"><span class="truncate font-medium text-gray-900 dark:text-white">{{ header.title }}</span><span v-if="header.is_default" class="badge badge-info">{{ t('invoice.header.default') }}</span></div><p class="mt-1 truncate text-sm text-gray-500 dark:text-gray-400">{{ header.tax_number || t('invoice.header.personal') }}</p></div>
          <button type="button" class="btn btn-secondary" @click="showHeaderManager = false; openHeaderEditor(header)"><Icon name="edit" size="sm" class="mr-1" />{{ t('common.edit') }}</button>
        </div>
        <div v-if="headers.length === 0" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('invoice.header.empty') }}</div>
      </div>
      <template #footer><div class="flex justify-between"><button type="button" class="btn btn-primary" @click="showHeaderManager = false; openHeaderEditor()"><Icon name="plus" size="sm" class="mr-1" />{{ t('invoice.header.add') }}</button><button type="button" class="btn btn-secondary" @click="showHeaderManager = false">{{ t('common.close') }}</button></div></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores'
import { invoiceAPI, type InvoiceApplication, type InvoiceEligibleOrder, type InvoiceHeader, type InvoiceHeaderInput } from '@/api/invoice'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const activeTab = ref<'apply' | 'history'>(route.query.tab === 'history' ? 'history' : 'apply')
const step = ref<'orders' | 'confirm'>('orders')
const loading = ref(false)
const historyLoading = ref(false)
const submitting = ref(false)
const headerSaving = ref(false)
const minAmount = ref(300)
const eligibleOrders = ref<InvoiceEligibleOrder[]>([])
const headers = ref<InvoiceHeader[]>([])
const applications = ref<InvoiceApplication[]>([])
const selectedOrderIDs = ref<number[]>([])
const selectedHeaderID = ref<number | null>(null)
const acknowledged = ref(false)
const showHeaderEditor = ref(false)
const showHeaderManager = ref(false)
const editingHeader = ref<InvoiceHeader | null>(null)
const viewingApplication = ref<InvoiceApplication | null>(null)
const historyPagination = reactive({ page: 1, page_size: 20, total: 0 })

const headerForm = reactive<InvoiceHeaderInput>({ title_type: 'personal', title: '', tax_number: '', email: '', phone: '', address: '', is_default: false })

const orderColumns = computed<Column[]>(() => [
  { key: 'select', label: '' }, { key: 'id', label: t('invoice.selectOrders.orderId') }, { key: 'order_no', label: t('invoice.selectOrders.orderNo') },
  { key: 'order_type', label: t('invoice.selectOrders.orderType') }, { key: 'amount', label: t('invoice.selectOrders.amount') }, { key: 'paid_at', label: t('invoice.selectOrders.paidAt') },
])
const applicationColumns = computed<Column[]>(() => [
  { key: 'application_no', label: t('invoice.history.applicationNo') }, { key: 'header_title', label: t('invoice.header.title') },
  { key: 'total_amount', label: t('invoice.history.amount') }, { key: 'status', label: t('invoice.history.status') },
  { key: 'created_at', label: t('invoice.history.createdAt') }, { key: 'actions', label: t('common.actions') },
])
const headerTypeOptions = computed(() => [
  { value: 'personal', label: t('invoice.header.personal') }, { value: 'company', label: t('invoice.header.company') },
])
const headerOptions = computed(() => [
  { value: null, label: t('invoice.header.choosePlaceholder') },
  ...headers.value.map(header => ({ value: header.id, label: header.tax_number ? `${header.title} (${header.tax_number})` : header.title })),
])
const selectedOrders = computed(() => eligibleOrders.value.filter(order => selectedOrderIDs.value.includes(order.id)))
const selectedTotal = computed(() => Number(selectedOrders.value.reduce((sum, order) => sum + Number(order.amount), 0).toFixed(2)))
const meetsMinimum = computed(() => selectedTotal.value >= minAmount.value)
const remainingAmount = computed(() => Math.max(0, Number((minAmount.value - selectedTotal.value).toFixed(2))))
const selectedHeader = computed(() => headers.value.find(header => header.id === selectedHeaderID.value) || null)

function formatCNY(value: number) { return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'CNY', minimumFractionDigits: 2 }).format(Number(value || 0)) }
function isSelected(id: number) { return selectedOrderIDs.value.includes(id) }
function toggleOrder(id: number) { selectedOrderIDs.value = isSelected(id) ? selectedOrderIDs.value.filter(value => value !== id) : [...selectedOrderIDs.value, id] }
function orderTypeLabel(type: string) { return t(`invoice.orderTypes.${type}`, type) }
function statusLabel(status: string) { return t(`invoice.status.${status}`, status) }
function statusClass(status: string) { return ({ PENDING: 'badge-warning', PROCESSING: 'badge-info', INVOICED: 'badge-success', REJECTED: 'badge-danger' } as Record<string, string>)[status] || 'badge-gray' }

async function loadData() {
  loading.value = true
  try {
    const [dataResponse, headersResponse] = await Promise.all([invoiceAPI.getApplicationData(), invoiceAPI.listHeaders()])
    minAmount.value = Number(dataResponse.data.min_amount || 300)
    eligibleOrders.value = dataResponse.data.orders || []
    headers.value = headersResponse.data || []
    selectedOrderIDs.value = selectedOrderIDs.value.filter(id => eligibleOrders.value.some(order => order.id === id))
    if (!selectedHeaderID.value || !headers.value.some(header => header.id === selectedHeaderID.value)) selectedHeaderID.value = headers.value.find(header => header.is_default)?.id || headers.value[0]?.id || null
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

async function loadApplications() {
  historyLoading.value = true
  try {
    const response = await invoiceAPI.listApplications({ page: historyPagination.page, page_size: historyPagination.page_size })
    applications.value = response.data.items || []
    historyPagination.total = response.data.total || 0
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error')))
  } finally {
    historyLoading.value = false
  }
}

async function submitApplication() {
  if (!selectedHeaderID.value || !meetsMinimum.value || !acknowledged.value) return
  submitting.value = true
  try {
    await invoiceAPI.createApplication({ order_ids: selectedOrderIDs.value, header_id: selectedHeaderID.value })
    appStore.showSuccess(t('invoice.messages.submitted'))
    selectedOrderIDs.value = []
    acknowledged.value = false
    step.value = 'orders'
    activeTab.value = 'history'
    await Promise.all([loadData(), loadApplications()])
    await router.replace({ path: '/invoices', query: { tab: 'history' } })
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error')))
  } finally {
    submitting.value = false
  }
}

function resetHeaderForm(header?: InvoiceHeader) {
  headerForm.title_type = header?.title_type || 'personal'
  headerForm.title = header?.title || ''
  headerForm.tax_number = header?.tax_number || ''
  headerForm.email = header?.email || ''
  headerForm.phone = header?.phone || ''
  headerForm.address = header?.address || ''
  headerForm.is_default = header?.is_default || false
}
function openHeaderEditor(header?: InvoiceHeader) { editingHeader.value = header || null; resetHeaderForm(header); showHeaderEditor.value = true }
function closeHeaderEditor() { showHeaderEditor.value = false; editingHeader.value = null; resetHeaderForm() }
async function saveHeader() {
  headerSaving.value = true
  try {
    const data = { ...headerForm }
    const response = editingHeader.value ? await invoiceAPI.updateHeader(editingHeader.value.id, data) : await invoiceAPI.createHeader(data)
    await loadData()
    selectedHeaderID.value = response.data.id
    closeHeaderEditor()
    appStore.showSuccess(t('common.success'))
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error')))
  } finally {
    headerSaving.value = false
  }
}
async function deleteHeader(header: InvoiceHeader) {
  if (!window.confirm(t('invoice.header.deleteConfirm', { title: header.title }))) return
  headerSaving.value = true
  try {
    await invoiceAPI.deleteHeader(header.id)
    await loadData()
    closeHeaderEditor()
    appStore.showSuccess(t('common.success'))
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error')))
  } finally {
    headerSaving.value = false
  }
}
async function openApplication(application: InvoiceApplication) {
  try {
    const response = await invoiceAPI.getApplication(application.id)
    viewingApplication.value = response.data
  } catch (error: unknown) { appStore.showError(extractI18nErrorMessage(error, t, 'invoice.errors', t('common.error'))) }
}
function changeHistoryPage(page: number) { historyPagination.page = page; loadApplications() }
function changeHistoryPageSize(size: number) { historyPagination.page_size = size; historyPagination.page = 1; loadApplications() }

onMounted(async () => {
  const preselectedOrderID = Number(route.query.order_id)
  await Promise.all([loadData(), loadApplications()])
  if (Number.isInteger(preselectedOrderID) && eligibleOrders.value.some(order => order.id === preselectedOrderID)) selectedOrderIDs.value = [preselectedOrderID]
})
</script>
