<template>
  <BaseDialog :show="!!selection" :title="dialogTitle" width="full" @close="emit('close')">
    <div class="flex h-[min(76dvh,850px)] max-h-[calc(90dvh-8rem)] min-h-0 flex-col gap-3">
      <div class="flex shrink-0 flex-wrap items-start gap-x-6 gap-y-1 border-b border-gray-200 text-sm dark:border-dark-700">
        <div class="flex shrink-0 gap-4" role="tablist" :aria-label="t('modelQuality.resultScope')">
          <button v-if="selection?.batchId" type="button" role="tab" data-scope="current" class="border-b-2 px-1 py-2" :class="scope === 'current' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" :aria-selected="scope === 'current'" @click="changeScope('current')">{{ t('modelQuality.currentResults') }}</button>
          <button type="button" role="tab" data-scope="history" class="border-b-2 px-1 py-2" :class="scope === 'history' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" :aria-selected="scope === 'history'" @click="changeScope('history')">{{ t('modelQuality.history') }}</button>
        </div>
        <div v-if="models.length > 1" class="flex min-w-0 flex-1 basis-full flex-wrap gap-x-3 gap-y-1 sm:basis-0" role="group" :aria-label="t('modelQuality.model')" data-model-switcher>
          <button v-for="item in models" :key="item" type="button" class="max-w-full break-all border-b-2 px-2 py-2 text-left transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500" :class="model === item ? 'border-primary-500 font-medium text-primary-600 dark:text-primary-400' : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'" :aria-pressed="model === item" :data-model-button="item" @click="changeModel(item)">{{ item }}</button>
        </div>
        <span v-else-if="models.length" class="min-w-0 break-all py-2 text-gray-500 dark:text-gray-400">{{ models[0] }}</span>
      </div>
      <div class="flex shrink-0 flex-wrap items-center justify-between gap-3">
        <div class="flex gap-1 border-b border-gray-200 dark:border-dark-700" role="tablist" :aria-label="t('modelQuality.resultTabs')">
          <button v-for="tab in tabs" :key="tab" type="button" role="tab" class="border-b-2 px-3 py-2 text-sm" :class="benchmark === tab ? 'border-primary-500 font-medium text-primary-600 dark:text-primary-400' : 'border-transparent text-gray-500'" :aria-selected="benchmark === tab" :data-benchmark-tab="tab" @click="changeTab(tab)">{{ qualityBenchmarkLabel(tab, t) }}</button>
        </div>
        <div class="flex items-center gap-3 text-xs text-gray-500">
          <label class="flex items-center gap-2"><span>{{ t('modelQuality.cardZoom') }}</span><input v-model.number="size" type="range" min="180" max="420" step="20" class="w-20 accent-primary-500" :aria-label="t('modelQuality.cardZoom')" /></label>
          <button type="button" class="btn btn-ghost btn-sm" :disabled="loading" @click="refresh">{{ t('modelQuality.refresh') }}</button>
        </div>
      </div>
      <p v-if="error" role="alert" class="shrink-0 text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <div ref="scroller" class="min-h-0 flex-1 overflow-y-auto pr-1" :aria-label="t('modelQuality.testResult')" :aria-busy="loading">
        <div v-if="loading && !cards.length" class="grid gap-3" :style="gridStyle"><div v-for="n in 8" :key="n" class="h-64 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700" /></div>
        <div v-else-if="cards.length" class="grid items-start gap-3 pb-1" :style="gridStyle">
          <ModelQualityResultCard v-for="run in cards" :key="run.id" :run="run" :highlighted="scope === 'current'" :body-height="`${Math.round(size * .8)}px`" @expand="expandedID = run.id; expandedMode = 'preview'" />
        </div>
        <p v-else class="py-16 text-center text-sm text-gray-500">{{ t('modelQuality.noRuns') }}</p>
      </div>
      <footer class="flex shrink-0 items-center justify-end gap-2 border-t border-gray-100 pt-2 text-xs text-gray-500 dark:border-dark-700">
        <button type="button" class="btn btn-ghost btn-sm" :disabled="loading || page <= 1" @click="changePage(-1)">{{ t('modelQuality.previousPage') }}</button>
        <span class="tabular-nums">{{ page }}</span>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="loading || !hasMore" @click="changePage(1)">{{ t('modelQuality.nextPage') }}</button>
      </footer>
    </div>
  </BaseDialog>
  <BaseDialog :show="!!expanded" :title="expanded ? `${expanded.target.model} · ${t(`modelQuality.verdicts.${qualityVerdict(expanded)}`)}` : ''" width="full" :z-index="60" @close="expandedID = ''">
    <div v-if="expanded" class="flex h-[min(70dvh,760px)] max-h-[calc(90dvh-8rem)] min-h-0 flex-col gap-3" data-quality-expanded>
      <div v-if="expanded.result?.html" class="flex shrink-0 gap-4 text-sm">
        <button type="button" :class="expandedMode === 'preview' ? 'font-medium text-primary-600' : 'text-gray-500'" @click="expandedMode = 'preview'">{{ t('modelQuality.preview') }}</button>
        <button type="button" :class="expandedMode === 'source' ? 'font-medium text-primary-600' : 'text-gray-500'" @click="expandedMode = 'source'">{{ t('modelQuality.sourceView') }}</button>
      </div>
      <div v-if="expanded.result?.html && expandedMode === 'preview'" class="min-h-0 flex-1">
        <ModelQualityPreview :html="expanded.result.html" :title="t('modelQuality.preview')" fill />
      </div>
      <pre v-else class="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words text-sm leading-6">{{ expanded.result?.html || expanded.result?.answer || expanded.reason }}</pre>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import * as api from '@/api/modelQuality'
import { extractApiErrorMessage } from '@/utils/apiError'
import ModelQualityResultCard from './ModelQualityResultCard.vue'
import ModelQualityPreview from './ModelQualityPreview.vue'
import { qualityBenchmarkLabel, qualityVerdict, type QualitySelection } from './qualityPresentation'

const props = withDefaults(defineProps<{ selection: QualitySelection | null; admin?: boolean }>(), { admin: false })
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const dialogTitle = computed(() => props.selection?.planName ? `${props.selection.planName} · ${t('modelQuality.testResult')}` : t('modelQuality.testResult'))
const rows = ref<api.QualityRun[]>([])
const tabs = ref<api.QualityBenchmark[]>([])
const models = ref<string[]>([])
const model = ref('')
const scope = ref<'current' | 'history'>('current')
const benchmark = ref<api.QualityBenchmark | ''>('')
const page = ref(1)
const hasMore = ref(false)
const loading = ref(false)
const error = ref('')
const size = ref(220)
const expandedID = ref('')
const expandedMode = ref<'preview' | 'source'>('preview')
const scroller = ref<HTMLElement>()
const gridStyle = computed(() => ({ gridTemplateColumns: `repeat(auto-fill, minmax(min(100%, ${size.value}px), 1fr))` }))
const cards = computed(() => rows.value.filter(r => !!r.finished_at))
const expanded = computed(() => cards.value.find(r => r.id === expandedID.value))
let request = 0
let disposed = false
let timer: ReturnType<typeof setTimeout> | undefined

function schedule() {
  if (timer) clearTimeout(timer)
  if (disposed || !props.selection || scope.value !== 'history') return
  timer = setTimeout(async () => {
    if (!document.hidden) await refresh()
    else schedule()
  }, 30000)
}
async function refresh() {
  const selection = props.selection
  if (!selection || disposed) return
  const seq = ++request
  loading.value = true
  error.value = ''
  try {
    const result = await api.planResults(props.admin, selection.planId, {
      ...(scope.value === 'current' ? { batch_id: selection.batchId } : { exclude_batch_id: selection.batchId }),
      model: model.value, benchmark: benchmark.value, page: page.value,
    })
    if (seq !== request || disposed) return
    rows.value = result.items
    models.value = result.models
    model.value = result.model
    tabs.value = result.benchmarks
    benchmark.value = result.benchmark
    hasMore.value = result.has_more
  } catch (e) {
    if (seq === request && !disposed) error.value = extractApiErrorMessage(e, t('modelQuality.loadFailed'))
  } finally {
    if (seq === request && !disposed) { loading.value = false; schedule() }
  }
}
function resetPage() {
  page.value = 1; rows.value = []; hasMore.value = false; expandedID.value = ''
  scroller.value?.scrollTo?.({ top: 0 })
}
function changeScope(value: 'current' | 'history') {
  if (scope.value === value) return
  scope.value = value
  tabs.value = []; models.value = []
  resetPage(); void refresh()
}
function changeModel(value: string) {
  if (model.value === value) return
  model.value = value
  tabs.value = []
  resetPage(); void refresh()
}
function changeTab(tab: api.QualityBenchmark) {
  if (benchmark.value === tab) return
  benchmark.value = tab
  resetPage(); void refresh()
}
function changePage(delta: number) {
  page.value += delta; rows.value = []; expandedID.value = ''
  scroller.value?.scrollTo?.({ top: 0 })
  void refresh()
}
watch([() => props.selection, () => props.admin], ([selection]) => {
  ++request
  if (timer) clearTimeout(timer)
  resetPage(); tabs.value = []; models.value = []; model.value = ''
  scope.value = selection?.batchId ? 'current' : 'history'
  benchmark.value = selection?.benchmark || ''
  error.value = ''; loading.value = false
  if (selection) void refresh()
}, { immediate: true })
onBeforeUnmount(() => { disposed = true; request++; if (timer) clearTimeout(timer) })
</script>
