<template>
  <div v-if="recent.length" class="min-w-0 border-t border-gray-200/70 pt-4 dark:border-dark-700" data-testid="quality-strip">
    <div class="min-w-0">
      <div class="mb-1 flex items-center justify-between gap-3 text-xs">
        <div class="flex min-w-0 items-center gap-2">
          <span class="shrink-0 font-semibold text-gray-800 dark:text-gray-100">{{ t('modelQuality.statusTitle') }}</span>
          <span class="min-w-0 truncate text-[11px] text-gray-500 dark:text-gray-400" :title="intervalDetails" data-quality-interval>{{ intervalSummary }}</span>
          <span v-if="showGroup" class="min-w-0 truncate text-gray-600 dark:text-gray-300" :title="groupNames">{{ groupNames }}</span>
        </div>
        <span class="flex shrink-0 items-center gap-1.5 text-gray-600 dark:text-gray-300">
          <span class="h-2 w-2 rounded-full" :class="qualityColor(statusRun)" aria-hidden="true" />
          {{ t(`modelQuality.verdicts.${qualitySummaryVerdict(statusSummary)}`) }}
        </span>
      </div>
      <div class="mb-2 flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <span class="min-w-0 flex-1 truncate text-xs text-gray-500 dark:text-gray-400" :title="planSummary">{{ planSummary }}</span>
        <ModelQualitySuccessRate :stats="recentStats" :label="t('modelQuality.last24hRate')" />
      </div>
      <div v-if="recent.length" class="flex h-5 flex-nowrap gap-[2px]" :aria-label="t('modelQuality.history')">
        <button
          v-for="{ batch, plan } in timeline"
          :key="batch.id"
          type="button"
          class="min-w-0 max-w-5 flex-1 rounded-sm transition-opacity hover:opacity-75 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500"
          :class="qualityColor(batch)"
          :data-run-id="batch.id"
          :title="batchLabel(batch, plan)"
          :aria-label="batchLabel(batch, plan)"
          @click.stop="emit('select', { planId: plan.id, batchId: batch.id, groupId: plan.group_id, groupName: plan.target.group_name, planName: plan.name })"
        />
      </div>
      <div v-else class="flex h-5 items-center text-xs text-gray-400">{{ t('modelQuality.noRuns') }}</div>
      <div v-if="recent.length" class="mt-1 flex justify-between text-[10px] tabular-nums text-gray-400">
        <span>{{ clock(recent[recent.length - 1].batch.started_at) }}</span>
        <span>{{ clock(recent[0].batch.started_at) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QualityBatch, QualityPlan, QualityStats } from '@/api/modelQuality'
import ModelQualitySuccessRate from './ModelQualitySuccessRate.vue'
import { qualityBenchmarkLabel, qualityColor, qualityTime, qualityVerdict, qualitySummaryVerdict, type QualitySelection } from './qualityPresentation'
const props = defineProps<{ plans: QualityPlan[]; showGroup?: boolean }>()
const emit = defineEmits<{ select: [selection: QualitySelection] }>()
const { t } = useI18n()
const byNewest = (a: QualityBatch, b: QualityBatch) => Date.parse(b.started_at) - Date.parse(a.started_at) || b.id.localeCompare(a.id)
const severity = ['degraded', 'error', 'unknown', 'cancelled', 'normal']
function completedBatches(plan: QualityPlan): QualityBatch[] {
  if (plan.batches) return plan.batches.filter(b => !!b.finished_at)
  // Compatibility with an older API response; never create pending blocks.
  return plan.recent.filter(r => !!r.finished_at).map(r => ({ id: r.batch_id || r.id, plan_id: plan.id, started_at: r.started_at, finished_at: r.finished_at!, verdict: r.verdict, count: 1 }))
}
const recent = computed(() => {
  const batches = new Map<string, { batch: QualityBatch; plan: QualityPlan }>()
  for (const plan of props.plans) {
    for (const batch of completedBatches(plan)) batches.set(batch.id, { batch, plan })
  }
  return [...batches.values()].sort((a, b) => byNewest(a.batch, b.batch)).slice(0, 60)
})
const timeline = computed(() => [...recent.value].reverse())
const groupNames = computed(() => [...new Set(props.plans.map(plan => plan.target.group_name))].join(' / '))
const planInterval = (plan: QualityPlan) => plan.enabled ? t('modelQuality.intervalShort', { minutes: plan.interval_minutes }) : t('modelQuality.manual')
const intervalDetails = computed(() => props.plans.map(plan => `${plan.name}：${planInterval(plan)}`).join(' / '))
const intervalSummary = computed(() => {
  const intervals = [...new Set(props.plans.map(planInterval))]
  return intervals.length === 1 ? intervals[0] : intervalDetails.value
})
const planSummary = computed(() => {
  const configs = props.plans.flatMap<{ model: string; benchmark: string }>(plan => plan.configs?.length ? plan.configs : [plan.target])
  const models = [...new Set(configs.map(c => c.model))]
  const benchmarks = [...new Set(configs.map(c => qualityBenchmarkLabel(c.benchmark, t)))]
  return `${models.join(' / ')} · ${benchmarks.join(' / ')}`
})
const latest = computed(() => props.plans.flatMap(plan => completedBatches(plan).sort(byNewest).slice(0, 1)))
const recentStats = computed<QualityStats | undefined>(() => {
  if (props.plans.some(plan => !plan.stats_24h)) return undefined
  return props.plans.reduce((sum, plan) => {
    for (const key of ['total', 'passed', 'failed', 'errors', 'unknown'] as const) sum[key] += plan.stats_24h![key]
    return sum
  }, { total: 0, passed: 0, failed: 0, errors: 0, unknown: 0 })
})
const latestStats = computed<QualityStats | undefined>(() => {
  if (!latest.value.length || latest.value.some(batch => !batch.stats)) return undefined
  return latest.value.reduce((total, batch) => {
    const stats = batch.stats!
    for (const key of ['total', 'passed', 'failed', 'errors', 'unknown'] as const) total[key] += stats[key]
    return total
  }, { total: 0, passed: 0, failed: 0, errors: 0, unknown: 0 })
})
const statusRun = computed(() => {
  for (const verdict of severity) {
    const batch = latest.value.find(b => qualityVerdict(b) === verdict)
    if (batch) return batch
  }
  return undefined
})
const statusSummary = computed(() => statusRun.value ? { verdict: statusRun.value.verdict, stats: latestStats.value } : undefined)
const batchLabel = (batch: QualityBatch, plan: QualityPlan) => {
  const label = `${qualityTime(batch.started_at)} · ${plan.name} · ${t('modelQuality.cardCount', { count: batch.count })} · ${t(`modelQuality.verdicts.${qualitySummaryVerdict(batch)}`)}`
  return label
}
const clock = (value: string) => new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
</script>
