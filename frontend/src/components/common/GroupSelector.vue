<template>
  <div>
    <label class="input-label">
      {{ t('admin.users.groups') }}
      <span class="font-normal text-gray-400">{{ t('common.selectedCount', { count: modelValue.length }) }}</span>
    </label>
    <div class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800">
      <div
        v-if="isSearchable"
        class="flex items-center gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-600"
      >
        <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
        <input
          v-model="searchText"
          type="text"
          :placeholder="t('common.searchPlaceholder')"
          class="min-w-0 flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-gray-100 dark:placeholder:text-dark-400"
        />
      </div>

      <div
        v-if="selectionActions && availableGroups.length > 0"
        class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 px-3 py-2 text-xs dark:border-dark-600"
      >
        <label class="flex cursor-pointer items-center gap-2 text-gray-700 dark:text-gray-300">
          <input
            type="checkbox"
            :checked="allFilteredSelected"
            :indeterminate="someFilteredSelected"
            :disabled="filteredGroups.length === 0"
            :aria-label="t('common.selectVisible')"
            class="h-3.5 w-3.5 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            @change="toggleFiltered(($event.target as HTMLInputElement).checked)"
          />
          <span>{{ t('common.selectVisible') }}</span>
          <span class="text-gray-400">{{ filteredSelectedCount }}/{{ filteredGroups.length }}</span>
        </label>
        <div class="flex items-center gap-1">
          <button
            type="button"
            :disabled="filteredGroups.length === 0"
            class="rounded px-2 py-1 font-medium text-gray-600 hover:bg-white disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-700"
            @click="invertFiltered"
          >
            {{ t('common.invertSelection') }}
          </button>
          <button
            type="button"
            :disabled="modelValue.length === 0"
            class="rounded px-2 py-1 font-medium text-gray-600 hover:bg-white disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-700"
            @click="clearSelection"
          >
            {{ t('common.clearSelection') }}
          </button>
        </div>
      </div>

      <div class="grid max-h-40 grid-cols-1 gap-1 overflow-y-auto p-2 sm:grid-cols-2">
        <label
          v-for="group in filteredGroups"
          :key="group.id"
          class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
          :title="t('admin.groups.rateAndAccounts', { rate: group.rate_multiplier, count: group.account_count || 0 })"
        >
          <input
            type="checkbox"
            :value="group.id"
            :checked="modelValue.includes(group.id)"
            @change="handleChange(group.id, ($event.target as HTMLInputElement).checked)"
            class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
          />
          <GroupBadge
            :name="group.name"
            :platform="group.platform"
            :subscription-type="group.subscription_type"
            :rate-multiplier="group.rate_multiplier"
            class="min-w-0 flex-1"
          />
          <span class="shrink-0 text-xs text-gray-400">{{ group.account_count || 0 }}</span>
        </label>
        <div
          v-if="filteredGroups.length === 0"
          class="py-2 text-center text-sm text-gray-500 dark:text-gray-400 sm:col-span-2"
        >
          {{ searchText.trim() ? t('common.noOptionsFound') : t('common.noGroupsAvailable') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from './GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminGroup, GroupPlatform } from '@/types'

const { t } = useI18n()

interface Props {
  modelValue: number[]
  groups: AdminGroup[]
  platform?: GroupPlatform // Optional platform filter
  mixedScheduling?: boolean // For antigravity accounts: allow anthropic/gemini groups
  searchable?: boolean | 'auto'
  selectionActions?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  searchable: 'auto',
  selectionActions: false
})
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const searchText = ref('')

// Filter groups by account platform before applying the optional text search.
const availableGroups = computed(() => {
  let result: AdminGroup[] = props.groups
  if (!props.platform) return result

  // antigravity 账户启用混合调度后，可选择 anthropic/gemini 分组
  if (props.platform === 'antigravity' && props.mixedScheduling) {
    return result.filter(
      (group) => group.platform === 'antigravity' || group.platform === 'anthropic' || group.platform === 'gemini' || group.platform === 'composite'
    )
  }

  // 默认：只能选择同 platform 的分组；composite 分组可接收任意具体平台账号
  return result.filter((group) => group.platform === props.platform || group.platform === 'composite')
})

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return availableGroups.value.length > 5
  return props.searchable
})

const filteredGroups = computed(() => {
  if (!isSearchable.value || !searchText.value.trim()) return availableGroups.value

  const query = searchText.value.trim().toLowerCase()
  return availableGroups.value.filter(
    (group) => group.name.toLowerCase().includes(query) || group.description?.toLowerCase().includes(query)
  )
})

const filteredSelectedCount = computed(() => {
  const selected = new Set(props.modelValue)
  return filteredGroups.value.reduce((count, group) => count + (selected.has(group.id) ? 1 : 0), 0)
})

const allFilteredSelected = computed(() => (
  filteredGroups.value.length > 0 && filteredSelectedCount.value === filteredGroups.value.length
))

const someFilteredSelected = computed(() => (
  filteredSelectedCount.value > 0 && !allFilteredSelected.value
))

const updateFilteredSelection = (mode: 'select' | 'remove' | 'invert') => {
  const selected = new Set(props.modelValue)
  for (const group of filteredGroups.value) {
    if (mode === 'select') selected.add(group.id)
    else if (mode === 'remove') selected.delete(group.id)
    else if (selected.has(group.id)) selected.delete(group.id)
    else selected.add(group.id)
  }
  emit('update:modelValue', Array.from(selected))
}

const toggleFiltered = (checked: boolean) => {
  updateFilteredSelection(checked ? 'select' : 'remove')
}

const invertFiltered = () => {
  updateFilteredSelection('invert')
}

const clearSelection = () => {
  emit('update:modelValue', [])
}

const handleChange = (groupId: number, checked: boolean) => {
  const selected = new Set(props.modelValue)
  if (checked) selected.add(groupId)
  else selected.delete(groupId)
  emit('update:modelValue', Array.from(selected))
}
</script>
