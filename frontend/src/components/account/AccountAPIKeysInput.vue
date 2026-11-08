<template>
  <div class="space-y-2" data-testid="account-api-keys">
    <p v-if="existingCount" class="input-hint">{{ t('admin.accounts.apiKeyPool.stored', { count: existingCount }) }}</p>
    <div v-for="(value, index) in keys" :key="index" class="flex items-center gap-2">
      <span v-if="keys.length > 1" class="w-5 shrink-0 text-right text-xs text-gray-500">{{ index + 1 }}</span>
      <input
        :value="value"
        type="password"
        :required="required"
        :placeholder="placeholder"
        :aria-label="t('admin.accounts.apiKeyPool.key', { index: index + 1 })"
        autocomplete="new-password"
        spellcheck="false"
        data-1p-ignore
        data-lpignore="true"
        data-bwignore="true"
        class="input min-w-0 flex-1 font-mono"
        @input="update(index, ($event.target as HTMLInputElement).value)"
        @paste="paste($event, index)"
      />
      <template v-if="keys.length > 1">
        <button type="button" class="btn btn-secondary px-2" :disabled="index === 0" :aria-label="t('admin.accounts.apiKeyPool.moveUp', { index: index + 1 })" @click="move(index, -1)">↑</button>
        <button type="button" class="btn btn-secondary px-2" :disabled="index === keys.length - 1" :aria-label="t('admin.accounts.apiKeyPool.moveDown', { index: index + 1 })" @click="move(index, 1)">↓</button>
        <button type="button" class="btn btn-secondary px-2" :aria-label="t('admin.accounts.apiKeyPool.remove', { index: index + 1 })" @click="remove(index)">×</button>
      </template>
    </div>
    <button type="button" class="btn btn-secondary text-sm" :disabled="keys.length >= MAX_ACCOUNT_API_KEYS" data-testid="add-account-api-key" @click="emit('update:modelValue', modelValue + '\n')">{{ t('admin.accounts.apiKeyPool.add') }}</button>
    <p class="input-hint">{{ t('admin.accounts.apiKeyPool.hint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MAX_ACCOUNT_API_KEYS } from './accountApiKeys'

const props = withDefaults(defineProps<{ modelValue: string; existingCount?: number; placeholder?: string; required?: boolean }>(), { existingCount: 0, placeholder: 'sk-...', required: false })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()
const keys = computed(() => props.modelValue.split(/\r?\n/))
function update(index: number, value: string) {
  const next = [...keys.value]
  next[index] = value
  emit('update:modelValue', next.join('\n'))
}
function move(index: number, direction: number) {
  const target = index + direction
  if (target < 0 || target >= keys.value.length) return
  const next = [...keys.value]
  const previous = next[index]
  next[index] = next[target]
  next[target] = previous
  emit('update:modelValue', next.join('\n'))
}
function remove(index: number) {
  emit('update:modelValue', keys.value.filter((_, position) => position !== index).join('\n'))
}
function paste(event: ClipboardEvent, index: number) {
  const value = event.clipboardData?.getData('text') ?? ''
  if (!/[\r\n]/.test(value.trim())) return
  event.preventDefault()
  const next = [...keys.value]
  next.splice(index, 1, ...value.trim().split(/\r?\n/))
  emit('update:modelValue', next.join('\n'))
}
</script>
