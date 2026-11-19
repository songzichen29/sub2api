<template>
  <div class="space-y-3" data-testid="account-api-key-editor">
    <div v-for="(entry, index) in modelValue" :key="entry.id ?? `new-${index}`" class="space-y-1">
      <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.accounts.apiKeyPool.key', { index: index + 1 }) }}<span v-if="entry.id" class="ml-2 font-mono">{{ entry.masked }}</span></span>
        <span v-if="entry.last_used_at">{{ t('admin.accounts.apiKeyPool.lastUsed') }} {{ formatDateTime(entry.last_used_at) }}</span>
      </div>
      <div class="flex items-center gap-2">
        <input
          :value="entry.value"
          type="password"
          autocomplete="new-password"
          spellcheck="false"
          data-1p-ignore
          data-lpignore="true"
          data-bwignore="true"
          :required="!entry.id"
          :placeholder="entry.id ? t('admin.accounts.apiKeyPool.replacePlaceholder') : 'sk-...'"
          :aria-label="t('admin.accounts.apiKeyPool.key', { index: index + 1 })"
          class="input min-w-0 flex-1 font-mono"
          @input="update(index, ($event.target as HTMLInputElement).value)"
          @paste="paste($event, index)"
        />
        <button type="button" class="btn btn-secondary px-2" :disabled="index === 0" :aria-label="t('admin.accounts.apiKeyPool.moveUp', { index: index + 1 })" @click="move(index, -1)">↑</button>
        <button type="button" class="btn btn-secondary px-2" :disabled="index === modelValue.length - 1" :aria-label="t('admin.accounts.apiKeyPool.moveDown', { index: index + 1 })" @click="move(index, 1)">↓</button>
        <button type="button" class="btn btn-secondary px-2" :disabled="modelValue.length === 1" :aria-label="t('admin.accounts.apiKeyPool.remove', { index: index + 1 })" @click="emit('update:modelValue', modelValue.filter((_, position) => position !== index))">×</button>
      </div>
    </div>
    <button type="button" class="btn btn-secondary btn-sm" :disabled="modelValue.length >= MAX_ACCOUNT_API_KEYS" data-testid="add-account-api-key" @click="emit('update:modelValue', [...modelValue, { value: '' }])">{{ t('admin.accounts.apiKeyPool.add') }}</button>
    <p v-if="validationError" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ t(`admin.accounts.apiKeyPool.errors.${validationError}`) }}</p>
    <p class="input-hint">{{ t('admin.accounts.apiKeyPool.editHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/utils/format'
import { accountApiKeyDraftError, MAX_ACCOUNT_API_KEYS, type AccountAPIKeyDraft } from './accountApiKeys'

const props = defineProps<{ modelValue: AccountAPIKeyDraft[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: AccountAPIKeyDraft[]] }>()
const { t } = useI18n()
const validationError = computed(() => accountApiKeyDraftError(props.modelValue))

function update(index: number, value: string) {
  emit('update:modelValue', props.modelValue.map((entry, position) => position === index ? { ...entry, value } : entry))
}

function move(index: number, offset: number) {
  const entries = [...props.modelValue]
  const [entry] = entries.splice(index, 1)
  entries.splice(index + offset, 0, entry)
  emit('update:modelValue', entries)
}

function paste(event: ClipboardEvent, index: number) {
  const value = event.clipboardData?.getData('text') ?? ''
  if (!/[\r\n]/.test(value.trim())) return
  event.preventDefault()
  const entries = [...props.modelValue]
  entries.splice(index, 1, ...value.trim().split(/\r?\n/).map(key => ({ value: key.trim() })))
  emit('update:modelValue', entries)
}
</script>
