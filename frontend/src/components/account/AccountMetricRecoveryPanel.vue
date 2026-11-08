<template>
  <section class="border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="account-metric-recovery-panel">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.metricRecovery.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.metricRecovery.hint') }}</p>
      </div>
      <Toggle v-model="enabled" :aria-label="t('admin.accounts.metricRecovery.enabled')" />
    </div>

    <div v-if="loading" class="mt-3 flex h-16 items-center justify-center text-gray-400">
      <Icon name="refresh" size="sm" class="animate-spin" />
    </div>

    <template v-else>
      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label v-for="metric in metricFields" :key="metric.key" class="block">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t(metric.label) }}</span>
          <div class="mt-1 grid grid-cols-2 gap-2">
            <input
              v-model.number="metric.pause"
              type="number"
              class="input"
              :min="metric.lower ? 0 : 1"
              :max="metric.lower ? 100 : 3600000"
              :step="metric.lower ? 0.1 : 1"
              :placeholder="t('admin.accounts.metricRecovery.disabled')"
              :aria-label="`${t(metric.label)} ${t('admin.accounts.metricRecovery.pause')}`"
            />
            <input
              v-model.number="metric.recovery"
              type="number"
              class="input"
              :min="metric.lower ? 0 : 1"
              :max="metric.lower ? 100 : 3600000"
              :step="metric.lower ? 0.1 : 1"
              :placeholder="t('admin.accounts.metricRecovery.recovery')"
              :aria-label="`${t(metric.label)} ${t('admin.accounts.metricRecovery.recovery')}`"
            />
          </div>
          <span class="mt-1 block text-[11px] text-gray-400">{{ t('admin.accounts.metricRecovery.pauseRecovery') }}</span>
        </label>
      </div>

      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="block">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.accounts.metricRecovery.cooldown') }}</span>
          <input v-model.number="cooldownSeconds" type="number" min="30" max="86400" step="30" class="input mt-1" />
        </label>
        <label class="block">
          <span class="text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('admin.accounts.metricRecovery.recoverySamples') }}</span>
          <input v-model.number="recoverySamples" type="number" min="1" max="30" step="1" class="input mt-1" />
        </label>
      </div>

      <div v-if="status?.monitor?.paused" class="mt-3 rounded border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="font-medium">{{ t('admin.accounts.metricRecovery.paused') }}</span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="release">
            <Icon name="refresh" size="xs" class="mr-1.5" />{{ t('admin.accounts.metricRecovery.release') }}
          </button>
        </div>
        <p v-if="status.monitor.reason" class="mt-1 break-words">{{ status.monitor.reason }}</p>
      </div>

      <div v-if="status?.metrics_available && status.metrics" class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 border-y border-gray-100 py-2 text-xs dark:border-dark-700">
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.metricRecovery.successRate') }}</span><span class="text-right text-gray-900 dark:text-white">{{ formatRate(status.metrics.success_rate) }}</span>
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.metricRecovery.cacheRate') }}</span><span class="text-right text-gray-900 dark:text-white">{{ formatRate(status.metrics.cache_rate) }}</span>
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.metricRecovery.ttft') }}</span><span class="text-right text-gray-900 dark:text-white">{{ formatMs(status.metrics.average_ttft_ms) }}</span>
        <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.metricRecovery.duration') }}</span><span class="text-right text-gray-900 dark:text-white">{{ formatMs(status.metrics.average_duration_ms) }}</span>
      </div>

      <p v-if="errorMessage" class="mt-2 text-xs text-rose-600 dark:text-rose-300">{{ errorMessage }}</p>
      <div class="mt-3 flex items-center justify-between gap-2">
        <span class="text-[11px] text-gray-400">{{ t('admin.accounts.metricRecovery.revision', { revision: status?.monitor?.revision ?? 0 }) }}</span>
        <button type="button" class="btn btn-primary btn-sm" :disabled="saving || loading" @click="save">
          <Icon name="check" size="xs" class="mr-1.5" />{{ t('common.save') }}
        </button>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AccountMetricRecoveryConfig, AccountPerformanceStatus } from '@/types'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ accountId: number }>()
const { t } = useI18n()
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const status = ref<AccountPerformanceStatus | null>(null)
const enabled = ref(false)
const cooldownSeconds = ref(600)
const recoverySamples = ref(3)

type MetricField = { key: 'success_rate' | 'cache_rate' | 'ttft_ms' | 'duration_ms'; label: string; lower: boolean; pause: number | null; recovery: number | null; minSamples: number }
const metricFields = reactive<MetricField[]>([
  { key: 'success_rate', label: 'admin.accounts.metricRecovery.successRate', lower: true, pause: null, recovery: null, minSamples: 10 },
  { key: 'cache_rate', label: 'admin.accounts.metricRecovery.cacheRate', lower: true, pause: null, recovery: null, minSamples: 10 },
  { key: 'ttft_ms', label: 'admin.accounts.metricRecovery.ttft', lower: false, pause: null, recovery: null, minSamples: 10 },
  { key: 'duration_ms', label: 'admin.accounts.metricRecovery.duration', lower: false, pause: null, recovery: null, minSamples: 10 }
])

function resetForm(next: AccountPerformanceStatus) {
  status.value = next
  const config = next.monitor?.config
  enabled.value = config?.enabled === true
  cooldownSeconds.value = config?.cooldown_seconds ?? 600
  recoverySamples.value = config?.recovery_samples ?? 3
  for (const field of metricFields) {
    const rule = config?.[field.key]
    field.pause = rule?.pause_threshold == null ? null : field.lower ? rule.pause_threshold * 100 : rule.pause_threshold
    field.recovery = rule?.recovery_threshold == null ? null : field.lower ? rule.recovery_threshold * 100 : rule.recovery_threshold
    field.minSamples = rule?.min_samples ?? 10
  }
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    resetForm(await adminAPI.accounts.getMetricRecovery(props.accountId))
  } catch (error: any) {
    errorMessage.value = error?.message || t('admin.accounts.metricRecovery.loadFailed')
  } finally {
    loading.value = false
  }
}

function buildConfig(): AccountMetricRecoveryConfig {
  const config: AccountMetricRecoveryConfig = {
    enabled: enabled.value,
    cooldown_seconds: Math.round(cooldownSeconds.value || 600),
    recovery_samples: Math.round(recoverySamples.value || 3)
  }
  for (const field of metricFields) {
    if (field.pause == null || !Number.isFinite(field.pause)) continue
    const pause = field.lower ? field.pause / 100 : field.pause
    const recovery = field.recovery == null || !Number.isFinite(field.recovery)
      ? pause
      : field.lower ? field.recovery / 100 : field.recovery
    config[field.key] = { pause_threshold: pause, recovery_threshold: recovery, min_samples: Math.round(field.minSamples || 10) }
  }
  return config
}

async function save() {
  if (!status.value) return
  saving.value = true
  errorMessage.value = ''
  try {
    const monitor = await adminAPI.accounts.saveMetricRecovery(props.accountId, status.value.monitor.revision, buildConfig())
    status.value = { ...status.value, monitor }
  } catch (error: any) {
    errorMessage.value = error?.message || t('admin.accounts.metricRecovery.saveFailed')
  } finally {
    saving.value = false
  }
}

async function release() {
  if (!status.value?.monitor) return
  saving.value = true
  errorMessage.value = ''
  try {
    const monitor = await adminAPI.accounts.releaseMetricRecovery(
      props.accountId,
      status.value.monitor.revision,
      status.value.monitor.pause_generation
    )
    status.value = { ...status.value, monitor }
    enabled.value = false
  } catch (error: any) {
    errorMessage.value = error?.message || t('admin.accounts.metricRecovery.releaseFailed')
  } finally {
    saving.value = false
  }
}

function formatRate(value: number | null | undefined) {
  return value == null ? '—' : `${(value * 100).toFixed(1)}%`
}

function formatMs(value: number | null | undefined) {
  return value == null ? '—' : `${Math.round(value)} ms`
}

watch(() => props.accountId, load, { immediate: true })
</script>
