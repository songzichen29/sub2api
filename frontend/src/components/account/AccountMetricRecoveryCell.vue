<template>
  <button type="button" class="block min-w-40 text-left text-xs text-gray-600 hover:text-primary-600 dark:text-dark-300 dark:hover:text-primary-400" :title="t('admin.accounts.metricRecovery.configure')" @click="$emit('configure')">
    <span v-if="loading && !status">{{ t('common.loading') }}</span>
    <span v-else-if="error && !status" class="text-rose-600 dark:text-rose-300">{{ t('admin.accounts.metricRecovery.loadFailed') }}</span>
    <template v-else-if="status">
      <span class="block font-medium" :class="status.monitor.paused ? 'text-amber-700 dark:text-amber-300' : ''" :title="status.monitor.reason">{{ stateLabel }}</span>
      <template v-if="status.metrics_available">
        <span class="block">{{ t('admin.accounts.metricRecovery.successShort') }} {{ rate(status.metrics?.success_rate) }} · {{ t('admin.accounts.metricRecovery.cacheShort') }} {{ rate(status.metrics?.cache_rate) }}</span>
        <span class="block">{{ t('admin.accounts.metricRecovery.ttftShort') }} {{ ms(status.metrics?.average_ttft_ms) }} · {{ t('admin.accounts.metricRecovery.durationShort') }} {{ ms(status.metrics?.average_duration_ms) }}</span>
        <span v-if="error" class="block text-rose-600 dark:text-rose-300">{{ t('admin.accounts.metricRecovery.loadFailed') }}</span>
        <span v-else class="block text-gray-400">{{ t('admin.accounts.metricRecovery.lastHour') }}</span>
      </template>
      <span v-else-if="error" class="block text-rose-600 dark:text-rose-300">{{ t('admin.accounts.metricRecovery.loadFailed') }}</span>
      <span v-else class="block text-gray-400">{{ t('admin.accounts.metricRecovery.unavailable') }}</span>
    </template>
    <span v-else>{{ t('admin.accounts.metricRecovery.configure') }}</span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountPerformanceStatus } from '@/types'
const props = defineProps<{ status?: AccountPerformanceStatus; loading?: boolean; error?: boolean }>()
defineEmits<{ configure: [] }>()
const { t } = useI18n()
const stateLabel = computed(() => {
  const monitor = props.status?.monitor
  const key = monitor?.paused ? (monitor.trial_started_at ? 'trial' : 'paused') : monitor?.config.enabled ? 'enabled' : 'notEnabled'
  return t(`admin.accounts.metricRecovery.${key}`)
})
const rate = (value?: number | null) => value == null ? '—' : `${(value * 100).toFixed(1)}%`
const ms = (value?: number | null) => value == null ? '—' : `${Math.round(value)} ms`
</script>
