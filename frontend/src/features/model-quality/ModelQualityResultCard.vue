<template>
  <article v-if="run.finished_at" class="min-w-0 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" :data-result-card="run.id" :aria-current="highlighted ? 'true' : undefined">
    <header class="border-b border-gray-200 bg-gray-50/80 px-3 py-3 dark:border-dark-700 dark:bg-dark-900/40">
      <div class="flex items-center justify-between gap-2">
        <h4 class="min-w-0 truncate text-xs font-medium text-gray-800 dark:text-gray-100" :title="cardName">{{ cardName }}</h4>
        <span class="min-w-0 truncate text-right text-[11px] text-gray-500 dark:text-gray-400" :title="modelLabel">{{ modelLabel }}</span>
      </div>
      <div class="mt-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-1">
        <ModelQualitySuccessRate :stats="run.stats_24h || emptyStats" :label="t('modelQuality.last24hRate')" :show-count="false" />
        <span class="inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium" :class="verdictClass" :title="run.reason"><span class="h-1.5 w-1.5 rounded-full" :class="qualityColor(run)" />{{ t(`modelQuality.verdicts.${qualityVerdict(run)}`) }}</span>
      </div>
    </header>
      <div v-if="run.result?.html && mode === 'preview'" :style="{ height: bodyHeight }">
        <ModelQualityPreview :html="run.result.html" :title="`${run.target.model} · ${t('modelQuality.preview')}`" fill />
      </div>
      <pre v-else-if="run.result?.html" class="overflow-auto whitespace-pre-wrap break-words bg-gray-50 p-3 text-[11px] leading-5 dark:bg-dark-900" :style="{ height: bodyHeight }">{{ run.result.html }}</pre>
      <div v-else-if="run.result?.answer" class="quality-answer overflow-auto p-3 text-xs leading-6" :style="{ height: bodyHeight }" v-html="answerHTML" />
      <div v-else class="flex items-center justify-center p-3 text-center text-xs leading-5 text-gray-500" :style="{ height: bodyHeight }">{{ run.reason || t('modelQuality.noResult') }}</div>
      <div class="flex items-start gap-3 border-t border-gray-100 px-3 py-1.5 text-[11px] dark:border-dark-700">
        <button v-if="run.result?.html" type="button" class="shrink-0 whitespace-nowrap text-primary-600 hover:underline dark:text-primary-400" :data-source-toggle="run.id" @click="mode = mode === 'preview' ? 'source' : 'preview'">{{ t(mode === 'preview' ? 'modelQuality.sourceView' : 'modelQuality.preview') }}</button>
        <button v-if="run.result?.html || run.result?.answer" type="button" class="shrink-0 whitespace-nowrap text-primary-600 hover:underline dark:text-primary-400" :data-expand-run="run.id" @click="emit('expand', run)">{{ t('modelQuality.expand') }}</button>
        <details class="ml-auto min-w-0 text-gray-500">
          <summary class="cursor-pointer text-right">{{ t('modelQuality.more') }}</summary>
          <div class="space-y-1 break-words pt-2 text-left leading-5">
            <p v-if="run.reason">{{ run.reason }}</p>
            <p>{{ t('modelQuality.source') }}: {{ run.result?.source || t('modelQuality.notReported') }}</p>
            <template v-if="run.result?.assessment">
              <p>{{ t('modelQuality.judgeModel') }}: {{ run.result.assessment.model }} · {{ t('modelQuality.evidenceConfidence') }} {{ Math.round(run.result.assessment.confidence * 100) }}%</p>
              <p v-for="check in run.result.assessment.checks" :key="check.id">{{ t(`modelQuality.checks.${check.id}`) }} {{ check.score }}/2: {{ check.evidence }}</p>
              <p>{{ t('modelQuality.judgeUsage') }} {{ run.result.judge_input_tokens ?? '—' }} / {{ run.result.judge_output_tokens ?? '—' }}</p>
            </template>
            <p>{{ t('modelQuality.inputTokens') }} {{ run.result?.input_tokens ?? '—' }} · {{ t('modelQuality.outputTokens') }} {{ run.result?.output_tokens ?? '—' }}</p>
            <p>{{ t('modelQuality.reasoningTokens') }} {{ run.result?.reasoning_tokens ?? '—' }}</p>
          </div>
        </details>
      </div>
    <footer class="flex items-center justify-between gap-2 border-t border-gray-200 bg-gray-50/80 px-3 py-2 text-[10px] tabular-nums text-gray-500 dark:border-dark-700 dark:bg-dark-900/40">
      <time>{{ qualityTime(run.started_at) }}</time>
      <span>{{ run.result?.duration_ms != null ? `${(run.result.duration_ms / 1000).toFixed(1)}s` : '—' }}</span>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import type { QualityRun } from '@/api/modelQuality'
import ModelQualityPreview from './ModelQualityPreview.vue'
import ModelQualitySuccessRate from './ModelQualitySuccessRate.vue'
import { qualityColor, qualityTime, qualityVerdict } from './qualityPresentation'

const props = defineProps<{ run: QualityRun; planName?: string; highlighted?: boolean; bodyHeight: string }>()
const emit = defineEmits<{ expand: [run: QualityRun] }>()
const { t } = useI18n()
const mode = ref<'preview' | 'source'>('preview')
const emptyStats = { total: 0, passed: 0, failed: 0, errors: 0, unknown: 0 }
const cardName = computed(() => props.planName || props.run.target.monitor_name || props.run.target.channel_name || props.run.target.model)
const modelLabel = computed(() => [props.run.target.model, props.run.target.reasoning_effort, props.run.target.service_tier].filter(Boolean).join(' · '))
const verdictClass = computed(() => {
  switch (qualityVerdict(props.run)) {
    case 'normal': return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'degraded': return 'bg-yellow-50 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300'
    default: return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
})
const answerHTML = computed(() => DOMPurify.sanitize(marked.parse(props.run.result?.answer || '', { async: false, breaks: true }), {
  ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'ul', 'ol', 'li', 'pre', 'code', 'blockquote', 'h1', 'h2', 'h3', 'h4', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'hr'],
  ALLOWED_ATTR: [],
}))
</script>

<style scoped>
.quality-answer :deep(p + p), .quality-answer :deep(ul), .quality-answer :deep(ol) { margin-top: .5rem; }
.quality-answer :deep(ul), .quality-answer :deep(ol) { padding-left: 1rem; list-style: revert; }
.quality-answer :deep(pre) { white-space: pre-wrap; overflow-wrap: anywhere; }
.quality-answer :deep(h1), .quality-answer :deep(h2), .quality-answer :deep(h3) { font-size: inherit; font-weight: 600; }
.quality-answer { overflow-wrap: anywhere; }
</style>
