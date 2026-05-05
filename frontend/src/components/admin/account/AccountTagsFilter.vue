<template>
  <div ref="rootEl" class="relative w-44">
    <!-- Trigger button：仿 Select 风格，左侧显示当前选中数，右侧 chevron。 -->
    <button
      type="button"
      class="flex w-full items-center justify-between rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-700 hover:border-gray-400 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-200 dark:hover:border-dark-400"
      :title="triggerTitle"
      @click="toggle"
    >
      <span class="truncate">{{ triggerLabel }}</span>
      <svg class="h-4 w-4 shrink-0 text-gray-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    <div
      v-if="open"
      class="absolute left-0 right-0 z-50 mt-1 rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
    >
      <!-- 搜索框：标签字典较多时方便定位。 -->
      <div class="border-b border-gray-200 p-2 dark:border-dark-600">
        <input
          v-model="search"
          type="text"
          class="input w-full text-sm"
          :placeholder="t('admin.accounts.tags.filterSearchPlaceholder')"
        />
      </div>

      <ul class="max-h-56 overflow-auto py-1">
        <li v-if="filteredTags.length === 0" class="px-3 py-2 text-center text-xs text-gray-500">
          {{ t('admin.accounts.tags.filterEmpty') }}
        </li>
        <li v-for="tag in filteredTags" :key="tag">
          <button
            type="button"
            class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
            @click="toggleTag(tag)"
          >
            <span
              :class="[
                'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                modelValue.includes(tag)
                  ? 'border-primary-500 bg-primary-500 text-white'
                  : 'border-gray-300 dark:border-dark-500'
              ]"
            >
              <svg v-if="modelValue.includes(tag)" class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            </span>
            <span class="truncate text-gray-900 dark:text-white">{{ tag }}</span>
          </button>
        </li>
      </ul>

      <div
        v-if="modelValue.length > 0"
        class="flex items-center justify-between border-t border-gray-200 px-3 py-2 text-xs dark:border-dark-600"
      >
        <span class="text-gray-500">{{ t('admin.accounts.tags.filterSelectedCount', { count: modelValue.length }) }}</span>
        <button type="button" class="text-primary-600 hover:underline dark:text-primary-400" @click="clearAll">
          {{ t('admin.accounts.tags.filterClear') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { onClickOutside } from '@vueuse/core'
import { useI18n } from 'vue-i18n'

interface Props {
  /** 当前选中的标签数组（AND 语义）。 */
  modelValue: string[]
  /** 候选标签字典——通常由父组件通过 listTags() 拉取后传入。 */
  availableTags?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  availableTags: () => []
})

const emit = defineEmits<{
  (event: 'update:modelValue', tags: string[]): void
  /** AccountTableFilters / AccountsView 用 change 触发列表 reload，与其他筛选器一致。 */
  (event: 'change'): void
}>()

const { t } = useI18n()

const rootEl = ref<HTMLElement | null>(null)
const open = ref(false)
const search = ref('')

onClickOutside(rootEl, () => {
  open.value = false
})

const filteredTags = computed(() => {
  const kw = search.value.trim().toLowerCase()
  if (kw === '') return props.availableTags
  return props.availableTags.filter(tag => tag.includes(kw))
})

const triggerLabel = computed(() => {
  if (props.modelValue.length === 0) {
    return t('admin.accounts.tags.filterAll')
  }
  if (props.modelValue.length === 1) {
    return props.modelValue[0]
  }
  return t('admin.accounts.tags.filterSelectedLabel', { count: props.modelValue.length })
})

const triggerTitle = computed(() => {
  if (props.modelValue.length === 0) return t('admin.accounts.tags.filterAll')
  return props.modelValue.join(', ')
})

function toggle() {
  open.value = !open.value
}

function toggleTag(tag: string) {
  const next = props.modelValue.includes(tag)
    ? props.modelValue.filter(t => t !== tag)
    : [...props.modelValue, tag]
  emit('update:modelValue', next)
  emit('change')
}

function clearAll() {
  emit('update:modelValue', [])
  emit('change')
}
</script>
