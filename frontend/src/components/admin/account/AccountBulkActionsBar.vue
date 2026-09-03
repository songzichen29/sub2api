<template>
  <div class="mb-4 flex flex-col gap-3 rounded-lg bg-primary-50 p-3 dark:bg-primary-900/20 lg:flex-row lg:items-center lg:justify-between">
    <div class="flex flex-wrap items-center gap-2">
      <span v-if="allResultsSelected" class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkActions.selectedAll', { count: selectedIds.length }) }}
      </span>
      <span v-else-if="selectedIds.length > 0" class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkActions.selected', { count: selectedIds.length }) }}
      </span>
      <span v-else class="text-sm font-medium text-primary-900 dark:text-primary-100">
        {{ t('admin.accounts.bulkEdit.title') }}
      </span>
      <template v-if="selectedIds.length > 0">
        <button
          :disabled="isBusy"
          @click="$emit('select-page')"
          class="text-xs font-medium text-primary-700 hover:text-primary-800 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-300 dark:hover:text-primary-200"
        >
          {{ t('admin.accounts.bulkActions.selectCurrentPage') }}
        </button>
      </template>
      <template v-if="!allResultsSelected && totalResults > selectedIds.length">
        <span v-if="selectedIds.length > 0" class="text-gray-300 dark:text-primary-800">•</span>
        <button
          :disabled="selectingAll || isBusy"
          @click="$emit('select-all-results')"
          class="text-xs font-medium text-primary-700 hover:text-primary-800 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-300 dark:hover:text-primary-200"
        >
          {{
            selectingAll
              ? t('admin.accounts.bulkActions.selectingAll')
              : t('admin.accounts.bulkActions.selectAllResults', { count: totalResults })
          }}
        </button>
      </template>
      <template v-if="selectedIds.length > 0">
        <span class="text-gray-300 dark:text-primary-800">•</span>
        <button
          :disabled="isBusy"
          @click="$emit('clear')"
          class="text-xs font-medium text-primary-700 hover:text-primary-800 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-300 dark:hover:text-primary-200"
        >
          {{ t('admin.accounts.bulkActions.clear') }}
        </button>
      </template>
    </div>
    <div class="flex flex-wrap gap-2 lg:justify-end">
      <template v-if="selectedIds.length > 0">
        <button :disabled="isBusy" @click="$emit('edit-selected')" class="btn btn-primary btn-sm">{{ t('admin.accounts.bulkActions.edit') }}</button>
        <button :disabled="isBusy" @click="$emit('toggle-schedulable', true)" class="btn btn-success btn-sm">{{ actionLabel('enableScheduling', 'enable-scheduling') }}</button>
        <button :disabled="isBusy" @click="$emit('toggle-schedulable', false)" class="btn btn-warning btn-sm">{{ actionLabel('disableScheduling', 'disable-scheduling') }}</button>
        <button :disabled="isBusy" @click="$emit('reset-status')" class="btn btn-secondary btn-sm">{{ actionLabel('resetStatus', 'reset-status') }}</button>
        <button :disabled="isBusy" @click="$emit('refresh-token')" class="btn btn-secondary btn-sm">{{ actionLabel('refreshToken', 'refresh-token') }}</button>
        <button :disabled="isBusy" @click="$emit('probe-upstream-billing')" class="btn btn-secondary btn-sm">{{ actionLabel('probeUpstreamBilling', 'probe-upstream-billing') }}</button>
        <button :disabled="isBusy" @click="$emit('delete')" class="btn btn-danger btn-sm">{{ actionLabel('delete', 'delete') }}</button>
      </template>
      <button :disabled="isBusy" @click="$emit('edit-filtered')" class="btn btn-secondary btn-sm">
        {{ busyAction === 'edit-filtered' ? t('common.processing') : t('admin.accounts.bulkActions.editFilteredResults') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  selectedIds: number[]
  totalResults: number
  selectingAll: boolean
  allResultsSelected: boolean
  busyAction?: string | null
}>()

defineEmits([
  'delete',
  'edit-selected',
  'edit-filtered',
  'clear',
  'select-page',
  'select-all-results',
  'toggle-schedulable',
  'reset-status',
  'refresh-token',
  'probe-upstream-billing'
])

const { t } = useI18n()
const isBusy = computed(() => Boolean(props.busyAction) || props.selectingAll)
const actionLabel = (labelKey: string, action: string) => (
  props.busyAction === action
    ? t('common.processing')
    : t(`admin.accounts.bulkActions.${labelKey}`)
)
</script>
