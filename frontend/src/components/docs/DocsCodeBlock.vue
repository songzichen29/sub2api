<template>
  <div class="overflow-hidden rounded-lg border border-gray-800 bg-gray-950">
    <div class="flex items-center justify-between gap-3 border-b border-gray-800 px-4 py-3">
      <span class="min-w-0 break-words font-mono text-xs text-gray-300">{{ label }}</span>
      <button
        type="button"
        data-testid="copy-code"
        class="inline-flex shrink-0 items-center gap-2 rounded px-2 py-1 text-xs text-gray-300 hover:bg-gray-800 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-400"
        :aria-label="t('docs.copyLabel', { label })"
        @click="copyToClipboard(code)"
      >
        <Icon :name="copied ? 'check' : 'copy'" size="sm" />
        <span aria-live="polite">{{ copied ? t('docs.copied') : t('docs.copy') }}</span>
      </button>
    </div>
    <pre class="overflow-x-auto p-4 font-mono text-[13px] leading-6 text-gray-100" tabindex="0" :aria-label="label"><code>{{ code }}</code></pre>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

defineProps<{ label: string; code: string }>()
const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()
</script>
