<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  opsAPI,
  type OpsErrorLogIngestionHealth,
  type OpsIngressRejectAggregate,
  type OpsIngressRejectHealth,
  type OpsIngressRejectQuery
} from '@/api/admin/ops'

const props = withDefaults(defineProps<{
  timeRange: string
  customStartTime?: string | null
  customEndTime?: string | null
  refreshToken?: number
}>(), {
  customStartTime: null,
  customEndTime: null,
  refreshToken: 0
})

const { t } = useI18n()
const loading = ref(false)
const loadError = ref('')
const ingestionHealth = ref<OpsErrorLogIngestionHealth | null>(null)
const rejectHealth = ref<OpsIngressRejectHealth | null>(null)
const rejects = ref<OpsIngressRejectAggregate[]>([])
const rejectTotal = ref(0)
let requestSequence = 0

const ingestionState = computed<'healthy' | 'warning' | 'stopped' | 'unknown'>(() => {
  const health = ingestionHealth.value
  if (!health) return 'unknown'
  if (!health.accepting) return 'stopped'
  if (health.dropped_count > 0 || health.write_failed_count > 0) return 'warning'
  return 'healthy'
})

const rejectState = computed<'healthy' | 'warning' | 'stopped' | 'unknown'>(() => {
  const health = rejectHealth.value
  if (!health) return 'unknown'
  if (!health.accepting) return 'stopped'
  if (health.dropped_count > 0 || health.flush_failure_count > 0 || health.overflowed_count > 0) return 'warning'
  return 'healthy'
})

function stateClass(state: 'healthy' | 'warning' | 'stopped' | 'unknown') {
  if (state === 'healthy') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (state === 'warning') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (state === 'stopped') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function buildRejectQuery(): OpsIngressRejectQuery {
  const params: OpsIngressRejectQuery = { page: 1, page_size: 10 }
  if (props.timeRange === 'custom' && props.customStartTime && props.customEndTime) {
    params.start_time = props.customStartTime
    params.end_time = props.customEndTime
  } else {
    params.time_range = props.timeRange === 'custom' ? '1h' : props.timeRange
  }
  return params
}

function formatTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  if (value < 1024) return `${Math.round(value)} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

function reasonLabel(reason: string) {
  const key = `admin.ops.errorIngestion.reasons.${reason || 'other'}`
  const label = t(key)
  return label === key ? (reason || t('admin.ops.errorIngestion.reasons.other')) : label
}

async function refresh() {
  const sequence = ++requestSequence
  loading.value = true
  loadError.value = ''

  const [ingestionResult, rejectHealthResult, rejectListResult] = await Promise.allSettled([
    opsAPI.getErrorLogIngestionHealth(),
    opsAPI.getIngressRejectHealth(),
    opsAPI.listIngressRejections(buildRejectQuery())
  ])
  if (sequence !== requestSequence) return

  const failed: string[] = []
  if (ingestionResult.status === 'fulfilled') ingestionHealth.value = ingestionResult.value
  else failed.push(t('admin.ops.errorIngestion.errorHealth'))

  if (rejectHealthResult.status === 'fulfilled') rejectHealth.value = rejectHealthResult.value
  else failed.push(t('admin.ops.errorIngestion.rejectHealth'))

  if (rejectListResult.status === 'fulfilled') {
    rejects.value = rejectListResult.value.items || []
    rejectTotal.value = rejectListResult.value.total || 0
  } else {
    failed.push(t('admin.ops.errorIngestion.rejectList'))
  }

  if (failed.length > 0) {
    loadError.value = t('admin.ops.errorIngestion.loadFailed', { targets: failed.join(', ') })
  }
  loading.value = false
}

watch(() => props.refreshToken, () => {
  void refresh()
})

onMounted(() => {
  void refresh()
})
</script>

<template>
  <section class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/60">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.errorIngestion.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorIngestion.description') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="refresh">
        {{ loading ? t('common.loading') : t('admin.ops.errorIngestion.refresh') }}
      </button>
    </div>

    <p v-if="loadError" class="mb-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300">
      {{ loadError }}
    </p>

    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      <div class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
        <div class="mb-3 flex items-center justify-between gap-2">
          <div class="text-xs font-semibold text-gray-800 dark:text-gray-200">{{ t('admin.ops.errorIngestion.errorPipeline') }}</div>
          <span data-testid="ingestion-state" class="rounded-md px-2 py-1 text-[11px] font-semibold" :class="stateClass(ingestionState)">
            {{ t(`admin.ops.errorIngestion.state.${ingestionState}`) }}
          </span>
        </div>
        <div v-if="ingestionHealth" class="flex flex-wrap gap-2 text-xs">
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
            {{ t('admin.ops.errorIngestion.queue') }} {{ ingestionHealth.queue_depth }}/{{ ingestionHealth.queue_capacity }}
          </span>
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
            {{ t('admin.ops.errorIngestion.queueBytes') }} {{ formatBytes(ingestionHealth.queue_bytes) }}/{{ formatBytes(ingestionHealth.queue_bytes_capacity) }}
          </span>
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ t('admin.ops.errorIngestion.enqueued') }} {{ ingestionHealth.enqueued_count }}</span>
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ t('admin.ops.errorIngestion.processed') }} {{ ingestionHealth.processed_count }}</span>
          <span class="rounded-md bg-emerald-100 px-2 py-1 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">{{ t('admin.ops.errorIngestion.persisted') }} {{ ingestionHealth.persisted_count }}</span>
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ t('admin.ops.errorIngestion.skipped') }} {{ ingestionHealth.skipped_count }}</span>
          <span class="rounded-md bg-amber-100 px-2 py-1 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ t('admin.ops.errorIngestion.dropped') }} {{ ingestionHealth.dropped_count }}</span>
          <span class="rounded-md bg-red-100 px-2 py-1 text-red-700 dark:bg-red-900/30 dark:text-red-300">{{ t('admin.ops.errorIngestion.failed') }} {{ ingestionHealth.write_failed_count }}</span>
        </div>
        <p v-if="ingestionHealth?.last_error" class="mt-3 break-all text-xs text-red-600 dark:text-red-400">
          {{ t('admin.ops.errorIngestion.latestWriteError') }} {{ ingestionHealth.last_error }}
          <span v-if="ingestionHealth.last_error_at" class="ml-1 text-gray-500 dark:text-gray-400">({{ formatTime(ingestionHealth.last_error_at) }})</span>
        </p>
      </div>

      <div class="rounded-xl border border-gray-200 p-3 dark:border-dark-700">
        <div class="mb-3 flex items-center justify-between gap-2">
          <div class="text-xs font-semibold text-gray-800 dark:text-gray-200">{{ t('admin.ops.errorIngestion.rejectPipeline') }}</div>
          <span data-testid="reject-state" class="rounded-md px-2 py-1 text-[11px] font-semibold" :class="stateClass(rejectState)">
            {{ t(`admin.ops.errorIngestion.state.${rejectState}`) }}
          </span>
        </div>
        <div v-if="rejectHealth" class="flex flex-wrap gap-2 text-xs">
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ t('admin.ops.errorIngestion.dimensions') }} {{ rejectHealth.cardinality }}/{{ rejectHealth.capacity }}</span>
          <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ t('admin.ops.errorIngestion.pending') }} {{ rejectHealth.pending_rows }}</span>
          <span class="rounded-md bg-emerald-100 px-2 py-1 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">{{ t('admin.ops.errorIngestion.flushed') }} {{ rejectHealth.flushed_request_count }}</span>
          <span class="rounded-md bg-amber-100 px-2 py-1 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ t('admin.ops.errorIngestion.overflowed') }} {{ rejectHealth.overflowed_count }}</span>
          <span class="rounded-md bg-amber-100 px-2 py-1 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">{{ t('admin.ops.errorIngestion.dropped') }} {{ rejectHealth.dropped_count }}</span>
          <span class="rounded-md bg-red-100 px-2 py-1 text-red-700 dark:bg-red-900/30 dark:text-red-300">{{ t('admin.ops.errorIngestion.failed') }} {{ rejectHealth.flush_failure_count }}</span>
        </div>
        <p v-if="rejectHealth?.last_error" class="mt-3 break-all text-xs text-red-600 dark:text-red-400">
          {{ t('admin.ops.errorIngestion.latestWriteError') }} {{ rejectHealth.last_error }}
        </p>
      </div>
    </div>

    <div class="mt-4 overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
      <div class="flex items-center justify-between border-b border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800/70">
        <div class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('admin.ops.errorIngestion.recentRejects') }}</div>
        <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorIngestion.total', { count: rejectTotal }) }}</div>
      </div>
      <div v-if="loading && rejects.length === 0" class="px-3 py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="rejects.length === 0" class="px-3 py-8 text-center text-sm text-gray-500">{{ t('admin.ops.errorIngestion.emptyRejects') }}</div>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 text-xs dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-gray-500 dark:bg-dark-800/70 dark:text-gray-400">
            <tr>
              <th class="px-3 py-2 font-medium">{{ t('admin.ops.errorIngestion.reason') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('admin.ops.errorIngestion.route') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('admin.ops.errorIngestion.identity') }}</th>
              <th class="px-3 py-2 font-medium">IP</th>
              <th class="px-3 py-2 text-right font-medium">{{ t('admin.ops.errorIngestion.count') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('admin.ops.errorIngestion.lastSeen') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 text-gray-700 dark:divide-dark-800 dark:text-gray-300">
            <tr v-for="item in rejects" :key="item.id || `${item.bucket_start}-${item.reject_reason}-${item.client_ip}`">
              <td class="whitespace-nowrap px-3 py-2 font-medium">{{ reasonLabel(item.reject_reason) }}</td>
              <td class="whitespace-nowrap px-3 py-2">{{ item.route_family }} / {{ item.protocol }}</td>
              <td class="whitespace-nowrap px-3 py-2">
                <span v-if="item.user_id">user #{{ item.user_id }}</span>
                <span v-if="item.user_id && item.api_key_id" class="mx-1 text-gray-400">/</span>
                <span v-if="item.api_key_id">key #{{ item.api_key_id }}</span>
                <span v-if="!item.user_id && !item.api_key_id">-</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2 font-mono">{{ item.client_ip || '-' }}</td>
              <td class="whitespace-nowrap px-3 py-2 text-right font-semibold">{{ item.request_count }}</td>
              <td class="whitespace-nowrap px-3 py-2">{{ formatTime(item.last_seen) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
