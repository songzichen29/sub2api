<template>
  <div v-if="plans.length" class="min-w-0 border-t border-gray-200/70 pt-4 dark:border-dark-700" data-testid="quality-strip">
    <div class="min-w-0">
      <div class="mb-1 flex items-center justify-between gap-3 text-xs">
        <span class="min-w-0 truncate font-semibold text-gray-800 dark:text-gray-100">{{ t('modelQuality.statusTitle') }}<span v-if="showGroup" class="ml-2 font-normal">{{ groupNames }}</span></span>
        <span class="flex shrink-0 items-center gap-1.5 text-gray-600 dark:text-gray-300">
          <span class="h-2 w-2 rounded-full" :class="qualityColor(statusRun)" aria-hidden="true" />
          {{ t(`modelQuality.verdicts.${qualityVerdict(statusRun)}`) }}
        </span>
      </div>
      <div class="mb-2 truncate text-xs text-gray-500 dark:text-gray-400" :title="planSummary">{{ planSummary }}</div>
      <div v-if="recent.length" class="flex h-5 flex-nowrap gap-[2px]" :aria-label="t('modelQuality.history')">
        <button
          v-for="{ run, plan } in timeline"
          :key="run.id"
          type="button"
          class="min-w-0 max-w-5 flex-1 rounded-sm transition-opacity hover:opacity-75 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500"
          :class="qualityColor(run)"
          :data-run-id="run.id"
          :title="runLabel(run)"
          :aria-label="runLabel(run)"
          @click.stop="emit('select', { groupId: plan.group_id, groupName: plan.target.group_name, planName: plan.name, runId: run.id })"
        />
      </div>
      <div v-else class="flex h-5 items-center text-xs text-gray-400">{{ t('modelQuality.noRuns') }}</div>
      <div v-if="recent.length" class="mt-1 flex justify-between text-[10px] tabular-nums text-gray-400">
        <span>{{ clock(recent[recent.length - 1].run.started_at) }}</span>
        <span>{{ clock(recent[0].run.started_at) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QualityPlan, QualityRun } from '@/api/modelQuality'
import { qualityBenchmarkLabel, qualityColor, qualityTime, qualityVerdict, type QualitySelection } from './qualityPresentation'
const props = defineProps<{ plans: QualityPlan[]; showGroup?: boolean }>()
const emit = defineEmits<{ select: [selection: QualitySelection] }>()
const { t } = useI18n()
const byNewest = (a: QualityRun, b: QualityRun) => Date.parse(b.started_at) - Date.parse(a.started_at) || b.id.localeCompare(a.id)
const recent = computed(() => {
  const runs = new Map<string, { run: QualityRun; plan: QualityPlan }>()
  for (const plan of props.plans) {
    for (const run of plan.recent) runs.set(run.id, { run, plan })
  }
  return [...runs.values()].sort((a, b) => byNewest(a.run, b.run)).slice(0, 60)
})
const timeline = computed(() => [...recent.value].reverse())
const groupNames = computed(() => [...new Set(props.plans.map(plan => plan.target.group_name))].join(' / '))
const planSummary = computed(() => {
  const models = [...new Set(props.plans.map(plan => `${plan.target.model} · ${plan.target.reasoning_effort}`))]
  const benchmarks = [...new Set(props.plans.map(plan => qualityBenchmarkLabel(plan.target.benchmark, t)))]
  return `${models.join(' / ')} · ${benchmarks.join(' / ')}`
})
const statusRun = computed(() => {
  const latest = props.plans.flatMap(plan => [...plan.recent].sort(byNewest).slice(0, 1))
  // A passing test of another type must not conceal a currently degraded type.
  for (const verdict of ['degraded', 'error', 'unknown', 'cancelled', 'pending', 'normal']) {
    const run = latest.find(run => qualityVerdict(run) === verdict)
    if (run) return run
  }
  return undefined
})
const runLabel = (run: QualityRun) => `${qualityTime(run.started_at)} · ${run.target.model} · ${qualityBenchmarkLabel(run.target.benchmark, t)} · ${t(`modelQuality.verdicts.${qualityVerdict(run)}`)}`
const clock = (value: string) => new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
</script>
