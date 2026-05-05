<template>
  <div>
    <div
      :class="[
        'flex flex-wrap items-center gap-1.5 rounded-lg border bg-white px-2 py-2 dark:bg-dark-700',
        focused
          ? 'border-primary-500 ring-1 ring-primary-500'
          : 'border-gray-300 dark:border-dark-500'
      ]"
      @click="focusInput"
    >
      <span
        v-for="tag in modelValue"
        :key="tag"
        class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-dark-600 dark:text-gray-300"
      >
        <span class="truncate">{{ tag }}</span>
        <button
          v-if="!disabled"
          type="button"
          class="shrink-0 rounded-full text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
          :aria-label="t('admin.accounts.tags.remove', { tag })"
          @click.stop="removeTag(tag)"
        >
          <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </span>

      <input
        ref="inputEl"
        v-model="draft"
        type="text"
        :disabled="disabled"
        :placeholder="modelValue.length === 0 ? placeholder : ''"
        class="min-w-[6rem] flex-1 bg-transparent text-sm text-gray-900 outline-none placeholder:text-gray-400 dark:text-white"
        @keydown="onKeydown"
        @focus="onFocus"
        @blur="onBlur"
      />
    </div>

    <!-- Suggestions dropdown：仅在聚焦且有 draft 时展示，避免空 dropdown 占位。 -->
    <div v-if="showDropdown && filteredSuggestions.length > 0" class="relative">
      <ul
        class="absolute left-0 right-0 z-50 mt-1 max-h-48 overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
        @mousedown.prevent
      >
        <li
          v-for="suggestion in filteredSuggestions"
          :key="suggestion"
          class="cursor-pointer px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-600"
          @click="addTag(suggestion)"
        >
          {{ suggestion }}
        </li>
      </ul>
    </div>

    <!-- 规范化失败时的内联错误提示。i18n 文案差异化展示长度/字符集/数量三类错误。 -->
    <p v-if="errorMessage" class="mt-1 text-xs text-red-600 dark:text-red-400">
      {{ errorMessage }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ACCOUNT_TAG_MAX_COUNT,
  ACCOUNT_TAG_MAX_LENGTH,
  type AccountTagError,
  normalizeAccountTag,
  validateAccountTag
} from '@/utils/accountTags'

interface Props {
  /** 当前已选标签数组，永远是规范化后的小写字典序数组。 */
  modelValue: string[]
  /** Autocomplete 候选列表；父组件用 listTags() 拉取后透传，组件本身不发请求。 */
  suggestions?: string[]
  placeholder?: string
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  suggestions: () => [],
  placeholder: '',
  disabled: false
})

const emit = defineEmits<{
  (event: 'update:modelValue', tags: string[]): void
  (event: 'invalid', error: AccountTagError): void
}>()

const { t } = useI18n()

const draft = ref('')
const focused = ref(false)
const error = ref<AccountTagError | null>(null)
const inputEl = ref<HTMLInputElement | null>(null)

const showDropdown = computed(() => focused.value && draft.value.trim() !== '')

const filteredSuggestions = computed(() => {
  const prefix = draft.value.trim().toLowerCase()
  if (prefix === '') return []
  const selected = new Set(props.modelValue)
  return props.suggestions
    .filter(s => !selected.has(s) && s.startsWith(prefix))
    .slice(0, 20)
})

const errorMessage = computed(() => {
  if (!error.value) return ''
  switch (error.value.code) {
    case 'INVALID_ACCOUNT_TAG_LENGTH':
      return t('admin.accounts.tags.errors.length', { max: ACCOUNT_TAG_MAX_LENGTH, tag: error.value.tag })
    case 'INVALID_ACCOUNT_TAG_CHARSET':
      return t('admin.accounts.tags.errors.charset', { tag: error.value.tag })
    case 'TOO_MANY_ACCOUNT_TAGS':
      return t('admin.accounts.tags.errors.count', { max: ACCOUNT_TAG_MAX_COUNT })
    default:
      return ''
  }
})

function focusInput() {
  if (props.disabled) return
  inputEl.value?.focus()
}

function onFocus() {
  focused.value = true
}

function onBlur() {
  // delay：让 dropdown 中的 click 事件能在 dropdown 消失前生效
  setTimeout(() => {
    focused.value = false
  }, 120)
}

function onKeydown(event: KeyboardEvent) {
  if (props.disabled) return
  if (event.key === 'Enter' || event.key === ',' || event.key === ' ') {
    event.preventDefault()
    if (draft.value.trim() === '') return
    addTag(draft.value)
  } else if (event.key === 'Backspace' && draft.value === '' && props.modelValue.length > 0) {
    event.preventDefault()
    removeTag(props.modelValue[props.modelValue.length - 1])
  }
}

/**
 * 把单个原始输入加到 modelValue。
 *
 * 流程：trim+lower 规范化 → 字符集/长度校验 → 去重 → 数量上限校验 → 排序后 emit。
 * 校验失败时 emit invalid 事件不修改 modelValue；上层应根据 invalid 事件做更
 * 显式的反馈（如 toast），而本组件内联的错误提示作为最低限度兜底。
 */
function addTag(input: string) {
  const tag = normalizeAccountTag(input)
  if (tag === null) {
    draft.value = ''
    return
  }
  const validationError = validateAccountTag(tag)
  if (validationError) {
    error.value = validationError
    emit('invalid', validationError)
    return
  }
  if (props.modelValue.includes(tag)) {
    draft.value = ''
    error.value = null
    return
  }
  if (props.modelValue.length >= ACCOUNT_TAG_MAX_COUNT) {
    const countError: AccountTagError = { code: 'TOO_MANY_ACCOUNT_TAGS', tag: '' }
    error.value = countError
    emit('invalid', countError)
    return
  }
  const next = [...props.modelValue, tag].sort()
  draft.value = ''
  error.value = null
  emit('update:modelValue', next)
}

function removeTag(tag: string) {
  if (props.disabled) return
  const next = props.modelValue.filter(t => t !== tag)
  emit('update:modelValue', next)
}
</script>
