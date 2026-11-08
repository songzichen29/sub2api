<template>
  <section class="border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="account-metric-recovery-panel">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.metricRecovery.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.metricRecovery.hint') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm shrink-0 whitespace-nowrap" :disabled="loading || saving" @click="load">{{ t('common.refresh') }}</button>
    </div>
    <p v-if="loading" class="py-4 text-sm text-gray-500">{{ t('common.loading') }}</p>
    <template v-else-if="status">
      <p v-if="!status.metrics_available" class="mt-3 text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.metricRecovery.unavailableHint') }}</p>
      <fieldset :disabled="saving || conflict" class="mt-3 space-y-4">
        <label class="flex items-center justify-between gap-2 text-sm">
          {{ t('admin.accounts.metricRecovery.enabled') }}
          <Toggle v-model="enabled" :disabled="(!status.metrics_available && !enabled) || saving || conflict" :aria-label="t('admin.accounts.metricRecovery.enabled')" />
        </label>
        <div v-for="metric in metricFields" :key="metric.key" class="border-t border-gray-100 pt-3 dark:border-dark-700">
          <h4 class="text-sm font-medium">{{ t(metric.label) }}</h4>
          <div class="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <label v-for="bound in boundFields" :key="bound.key" class="block text-xs text-gray-600 dark:text-gray-300">
              {{ t(bound.label) }}
              <input v-model="metric[bound.key]" type="number" class="input mt-1" min="0" :max="metric.rate ? 100 : 3600000" :step="metric.rate ? 0.1 : 1" :placeholder="t(bound.recovery ? 'admin.accounts.metricRecovery.inherit' : 'admin.accounts.metricRecovery.disabled')" :aria-label="t(metric.label) + ' ' + t(bound.label)" />
            </label>
          </div>
          <label class="mt-2 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.accounts.metricRecovery.minSamples') }}
            <input v-model.number="metric.minSamples" type="number" min="1" max="5000" step="1" class="input w-24" :aria-label="t(metric.label) + ' ' + t('admin.accounts.metricRecovery.minSamples')" />
          </label>
        </div>
        <p class="text-xs text-gray-500">{{ t('admin.accounts.metricRecovery.boundsHint') }}</p>
        <label class="flex items-center justify-between text-sm">
          {{ t('admin.accounts.metricRecovery.autoRecover') }}
          <Toggle v-model="autoRecover" :disabled="saving || conflict" :aria-label="t('admin.accounts.metricRecovery.autoRecover')" />
        </label>
        <div class="grid grid-cols-2 gap-3">
          <label class="block text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.accounts.metricRecovery.cooldown') }}
            <input v-model.number="cooldownSeconds" type="number" min="30" max="86400" step="1" class="input mt-1" />
          </label>
          <label class="block text-xs text-gray-600 dark:text-gray-300">
            {{ t('admin.accounts.metricRecovery.recoverySamples') }}
            <input v-model.number="recoverySamples" type="number" min="1" max="100" step="1" class="input mt-1" />
          </label>
        </div>
      </fieldset>

      <div v-if="status.monitor.paused" class="mt-3 rounded border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="font-medium">{{ t(status.monitor.trial_started_at ? 'admin.accounts.metricRecovery.trial' : 'admin.accounts.metricRecovery.paused') }}</span>
          <button type="button" data-testid="metric-release" class="btn btn-secondary btn-sm" :disabled="saving || conflict" @click="release">{{ t('admin.accounts.metricRecovery.release') }}</button>
        </div>
        <p v-if="status.monitor.reason" class="mt-1 break-words">{{ status.monitor.reason }}</p>
        <p v-if="status.monitor.config.auto_recover === false" class="mt-1">{{ t('admin.accounts.metricRecovery.manualOnly') }}</p>
        <p v-else-if="status.monitor.next_trial_at" class="mt-1">{{ t('admin.accounts.metricRecovery.nextTrial') }} {{ date(status.monitor.next_trial_at) }}</p>
        <p v-if="status.monitor.trial_started_at" class="mt-1">{{ t('admin.accounts.metricRecovery.trialBudget', { used: status.monitor.trial_requests, total: (status.monitor.config.recovery_samples ?? 3) * 3 }) }}</p>
      </div>

      <div v-if="status.metrics_available && status.metrics" class="mt-3 border-y border-gray-100 py-2 text-xs dark:border-dark-700">
        <p class="mb-2 text-gray-500">{{ t('admin.accounts.metricRecovery.evaluationWindow') }} {{ date(status.metrics.window_start) }} — {{ date(status.metrics.window_end) }}</p>
        <dl class="grid grid-cols-2 gap-x-4 gap-y-1">
          <template v-for="item in observations" :key="item.label">
            <dt class="text-gray-500 dark:text-gray-400">{{ t(item.label) }}</dt>
            <dd class="text-right">{{ item.value }} · {{ t('admin.accounts.metricRecovery.sampleCount', { count: item.samples }) }}</dd>
          </template>
        </dl>
      </div>
      <div class="mt-3 flex items-center justify-between gap-2">
        <span class="text-xs text-gray-400">{{ t('admin.accounts.metricRecovery.revision', { revision: status.monitor.revision }) }}</span>
        <button type="button" class="btn btn-primary btn-sm" :disabled="saving || conflict || (!status.metrics_available && enabled)" @click="save">{{ t('common.save') }}</button>
      </div>
      <p v-if="saved" role="status" class="mt-2 text-xs text-emerald-700 dark:text-emerald-300">{{ t('admin.accounts.metricRecovery.saved') }}</p>

      <div class="mt-4 border-t border-gray-200 pt-3 dark:border-dark-600">
        <h4 class="text-sm font-medium">{{ t('admin.accounts.metricRecovery.history') }}</h4>
        <p v-if="historyError" class="mt-2 text-xs text-rose-600">{{ historyError }} <button type="button" class="underline" @click="loadHistory(true)">{{ t('common.retry') }}</button></p>
        <p v-else-if="!events.length && !historyLoading" class="mt-2 text-xs text-gray-500">{{ t('admin.accounts.metricRecovery.noHistory') }}</p>
        <ol class="divide-y divide-gray-100 text-xs dark:divide-dark-700">
          <li v-for="event in events" :key="event.id" class="py-2">
            <div class="flex flex-wrap justify-between gap-2">
              <span>{{ t('admin.accounts.metricRecovery.actions.' + (event.action === 'trial' && event.evidence?.outcome ? 'trialEnd' : event.action)) }} · {{ t('admin.accounts.metricRecovery.revision', { revision: event.revision }) }}</span>
              <time>{{ date(event.created_at) }}</time>
            </div>
            <details class="mt-1 text-gray-500">
              <summary class="cursor-pointer">{{ t('admin.accounts.metricRecovery.evidence') }}</summary>
              <pre class="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-all">{{ JSON.stringify(event.evidence, null, 2) }}</pre>
            </details>
          </li>
        </ol>
        <button v-if="hasMore" type="button" class="btn btn-secondary btn-sm mt-2" :disabled="historyLoading" @click="loadHistory(false)">{{ t('admin.accounts.metricRecovery.moreHistory') }}</button>
        <p v-if="historyLoading" class="mt-2 text-xs text-gray-500">{{ t('common.loading') }}</p>
      </div>
    </template>
    <p v-if="errorMessage" role="alert" class="mt-2 text-xs text-rose-600 dark:text-rose-300">{{ errorMessage }}</p>
    <p v-if="conflict" class="mt-1 text-xs text-amber-700 dark:text-amber-300">{{ t('admin.accounts.metricRecovery.conflict') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AccountMetricRecoveryConfig, AccountPerformanceEvent, AccountPerformanceStatus } from '@/types'
import Toggle from '@/components/common/Toggle.vue'

const props = defineProps<{ accountId: number }>()
const emit = defineEmits<{ updated: [accountId: number] }>()
const { t } = useI18n()
const loading = ref(false)
const saving = ref(false)
const conflict = ref(false)
const saved = ref(false)
const errorMessage = ref('')
const status = ref<AccountPerformanceStatus | null>(null)
const enabled = ref(false)
const autoRecover = ref(true)
const cooldownSeconds = ref(300)
const recoverySamples = ref(3)
const events = ref<AccountPerformanceEvent[]>([])
const historyError = ref('')
const historyLoading = ref(false)
const hasMore = ref(false)
let epoch = 0
let historySequence = 0
const historyPageSize = 20

type MetricKey = 'success_rate' | 'cache_rate' | 'ttft_ms' | 'duration_ms'
type BoundKey = 'pauseMin' | 'pauseMax' | 'recoveryMin' | 'recoveryMax'
type MetricField = { key: MetricKey; label: string; rate: boolean; minSamples: number } & Record<BoundKey, string>
const metricFields = reactive<MetricField[]>([
  { key: 'success_rate', label: 'admin.accounts.metricRecovery.successRate', rate: true },
  { key: 'cache_rate', label: 'admin.accounts.metricRecovery.cacheRate', rate: true },
  { key: 'ttft_ms', label: 'admin.accounts.metricRecovery.ttft', rate: false },
  { key: 'duration_ms', label: 'admin.accounts.metricRecovery.duration', rate: false }
].map(field => ({ ...field, key: field.key as MetricKey, pauseMin: '', pauseMax: '', recoveryMin: '', recoveryMax: '', minSamples: 20 })))
const boundFields: { key: BoundKey; label: string; recovery: boolean }[] = [
  { key: 'pauseMin', label: 'admin.accounts.metricRecovery.pauseMin', recovery: false },
  { key: 'pauseMax', label: 'admin.accounts.metricRecovery.pauseMax', recovery: false },
  { key: 'recoveryMin', label: 'admin.accounts.metricRecovery.recoveryMin', recovery: true },
  { key: 'recoveryMax', label: 'admin.accounts.metricRecovery.recoveryMax', recovery: true }
]
const rate = (v?: number | null) => v == null ? '—' : `${(v * 100).toFixed(1)}%`
const ms = (v?: number | null) => v == null ? '—' : `${Math.round(v)} ms`
const date = (v: string) => new Date(v).toLocaleString()
const observations = computed(() => {
  const m = status.value?.metrics
  return [
    { label: metricFields[0].label, value: rate(m?.success_rate), samples: (m?.success_samples ?? 0) + (m?.error_samples ?? 0) },
    { label: metricFields[1].label, value: rate(m?.cache_rate), samples: m?.cache_samples ?? 0 },
    { label: metricFields[2].label, value: ms(m?.average_ttft_ms), samples: m?.ttft_samples ?? 0 },
    { label: metricFields[3].label, value: ms(m?.average_duration_ms), samples: m?.duration_samples ?? 0 }
  ]
})

function resetForm(next: AccountPerformanceStatus) {
  status.value = next
  const config = next.monitor.config
  enabled.value = config.enabled
  autoRecover.value = config.auto_recover !== false
  cooldownSeconds.value = config.cooldown_seconds || 300
  recoverySamples.value = config.recovery_samples || 3
  for (const field of metricFields) {
    const rule = config[field.key]
    const stringify = (v?: number) => v == null ? '' : String(Number((v * (field.rate ? 100 : 1)).toPrecision(12)))
    const pause = rule?.pause ?? (field.rate ? { min: rule?.pause_threshold } : { max: rule?.pause_threshold })
    const recovery = rule?.pause ? rule.recovery : (field.rate ? { min: rule?.recovery_threshold } : { max: rule?.recovery_threshold })
    field.pauseMin = stringify(pause.min)
    field.pauseMax = stringify(pause.max)
    field.recoveryMin = stringify(recovery?.min)
    field.recoveryMax = stringify(recovery?.max)
    field.minSamples = rule?.min_samples || 20
  }
}

async function load() {
  const current = ++epoch
  const id = props.accountId
  ++historySequence
  status.value = null
  events.value = []
  historyError.value = ''
  historyLoading.value = false
  hasMore.value = false
  loading.value = true
  saving.value = false
  conflict.value = false
  saved.value = false
  errorMessage.value = ''
  try {
    const next = await adminAPI.accounts.getMetricRecovery(id)
    if (current !== epoch) return
    resetForm(next)
    void loadHistory(true)
  } catch (error) {
    if (current === epoch) errorMessage.value = message(error, 'loadFailed')
  } finally {
    if (current === epoch) loading.value = false
  }
}

function buildConfig(): AccountMetricRecoveryConfig {
  const integer = (v: number, min: number, max: number) => {
    if (!Number.isInteger(v) || v < min || v > max) throw new Error(t('admin.accounts.metricRecovery.invalidConfig'))
    return v
  }
  const config: AccountMetricRecoveryConfig = {
    enabled: enabled.value, auto_recover: autoRecover.value,
    cooldown_seconds: integer(cooldownSeconds.value, 30, 86400),
    recovery_samples: integer(recoverySamples.value, 1, 100)
  }
  for (const field of metricFields) {
    const value = (raw: string) => {
      if (String(raw).trim() === '') return undefined
      const n = Number(raw)
      if (!Number.isFinite(n) || n < 0 || n > (field.rate ? 100 : 3600000)) throw new Error(t('admin.accounts.metricRecovery.invalidConfig'))
      return field.rate ? n / 100 : n
    }
    const pause = { min: value(field.pauseMin), max: value(field.pauseMax) }
    const recovery = { min: value(field.recoveryMin), max: value(field.recoveryMax) }
    if (pause.min == null && pause.max == null && recovery.min == null && recovery.max == null) continue
    config[field.key] = { pause, recovery, min_samples: integer(field.minSamples, 1, 5000) }
  }
  return config
}

function message(error: unknown, fallback: string) {
  const e = error as { message?: string; response?: { status?: number; data?: { message?: string } } }
  return e.response?.data?.message || e.message || t('admin.accounts.metricRecovery.' + fallback)
}
function writeFailed(error: unknown, fallback: string) {
  const failure = error as { status?: number; response?: { status?: number } }
  conflict.value = (failure.status ?? failure.response?.status) === 409
  errorMessage.value = message(error, fallback)
}

async function save() {
  if (!status.value || saving.value || conflict.value) return
  const current = epoch, id = props.accountId, previous = status.value
  saving.value = true; saved.value = false; errorMessage.value = ''
  try {
    const monitor = await adminAPI.accounts.saveMetricRecovery(id, previous.monitor.revision, buildConfig())
    emit('updated', id)
    if (current !== epoch) return
    resetForm({ ...previous, monitor, metrics: null })
    saved.value = true
    void loadHistory(true)
  } catch (error) {
    if (current === epoch) writeFailed(error, 'saveFailed')
  } finally {
    if (current === epoch) saving.value = false
  }
}

async function release() {
  if (!status.value || saving.value || conflict.value) return
  const current = epoch, id = props.accountId, previous = status.value
  saving.value = true; saved.value = false; errorMessage.value = ''
  try {
    const monitor = await adminAPI.accounts.releaseMetricRecovery(id, previous.monitor.revision, previous.monitor.pause_generation)
    emit('updated', id)
    if (current !== epoch) return
    resetForm({ ...previous, monitor, metrics: null })
    saved.value = true
    void loadHistory(true)
  } catch (error) {
    if (current === epoch) writeFailed(error, 'releaseFailed')
  } finally {
    if (current === epoch) saving.value = false
  }
}

async function loadHistory(reset: boolean) {
  const current = epoch, sequence = ++historySequence, id = props.accountId
  const before = reset ? undefined : events.value.at(-1)?.id
  historyLoading.value = true; historyError.value = ''
  try {
    const result = await adminAPI.accounts.listMetricRecoveryEvents(id, before, historyPageSize)
    if (current !== epoch || sequence !== historySequence) return
    events.value = reset ? result.events : [...events.value, ...result.events]
    hasMore.value = result.events.length === historyPageSize
  } catch (error) {
    if (current === epoch && sequence === historySequence) historyError.value = message(error, 'historyFailed')
  } finally {
    if (current === epoch && sequence === historySequence) historyLoading.value = false
  }
}
watch(() => props.accountId, load, { immediate: true })
onUnmounted(() => { ++epoch; ++historySequence })
</script>
