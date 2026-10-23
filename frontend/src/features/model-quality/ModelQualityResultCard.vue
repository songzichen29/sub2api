<template>
  <article v-if="run.finished_at" class="min-w-0 overflow-hidden rounded-lg border bg-white dark:bg-dark-800" :class="highlighted ? 'border-primary-500 ring-1 ring-primary-500/20' : 'border-gray-200 dark:border-dark-700'" :data-result-card="run.id" :aria-current="highlighted ? 'true' : undefined">
    <header class="border-b border-gray-100 px-3 py-2 dark:border-dark-700">
      <div class="flex items-center justify-between gap-2">
        <h4 class="min-w-0 truncate text-xs font-semibold" :title="run.target.model">{{ run.target.model }}</h4>
        <span class="flex shrink-0 items-center gap-1 text-[11px]" :title="run.reason"><span class="h-2 w-2 rounded-full" :class="qualityColor(run)" />{{ t(`modelQuality.verdicts.${qualityVerdict(run)}`) }}</span>
      </div>
      <div class="mt-1 truncate text-[11px] text-gray-500" :title="run.target.monitor_name || run.target.channel_name">{{ run.target.monitor_name || run.target.channel_name }} · {{ run.target.reasoning_effort }}</div>
    </header>
      <ModelQualityPreview v-if="run.result?.html && mode === 'preview'" :html="run.result.html" :title="`${run.target.model} · ${t('modelQuality.preview')}`" />
      <pre v-else-if="run.result?.html" class="overflow-auto whitespace-pre-wrap break-words bg-gray-50 p-3 text-[11px] leading-5 dark:bg-dark-900" :style="{ height: bodyHeight }">{{ run.result.html }}</pre>
      <div v-else-if="run.result?.answer" class="quality-answer overflow-auto p-3 text-xs leading-6" :style="{ height: bodyHeight }" v-html="answerHTML" />
      <div v-else class="flex items-center justify-center p-3 text-center text-xs leading-5 text-gray-500" :style="{ height: bodyHeight }">{{ run.reason || t('modelQuality.noResult') }}</div>
      <div class="flex items-center gap-3 border-t border-gray-100 px-3 py-1.5 text-[11px] dark:border-dark-700">
        <button v-if="run.result?.html" type="button" class="text-primary-600 hover:underline dark:text-primary-400" :data-source-toggle="run.id" @click="mode = mode === 'preview' ? 'source' : 'preview'">{{ t(mode === 'preview' ? 'modelQuality.sourceView' : 'modelQuality.preview') }}</button>
        <button v-if="run.result?.html || run.result?.answer" type="button" class="text-primary-600 hover:underline dark:text-primary-400" :data-expand-run="run.id" @click="emit('expand', run)">{{ t('modelQuality.expand') }}</button>
        <details class="ml-auto min-w-0 text-gray-500">
          <summary class="cursor-pointer text-right">{{ t('modelQuality.more') }}</summary>
          <div class="space-y-1 break-words pt-2 text-left leading-5">
            <p v-if="run.reason">{{ run.reason }}</p>
            <p>{{ t('modelQuality.inputTokens') }} {{ run.result?.input_tokens ?? '—' }} · {{ t('modelQuality.outputTokens') }} {{ run.result?.output_tokens ?? '—' }}</p>
            <p>{{ t('modelQuality.reasoningTokens') }} {{ run.result?.reasoning_tokens ?? '—' }}</p>
          </div>
        </details>
      </div>
    <footer class="flex items-center justify-between gap-2 border-t border-gray-100 px-3 py-2 text-[10px] tabular-nums text-gray-500 dark:border-dark-700">
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
import { qualityColor, qualityTime, qualityVerdict } from './qualityPresentation'

const props = defineProps<{ run: QualityRun; highlighted?: boolean; bodyHeight: string }>()
const emit = defineEmits<{ expand: [run: QualityRun] }>()
const { t } = useI18n()
const mode = ref<'preview' | 'source'>('preview')
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
