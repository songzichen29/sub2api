<template>
  <main class="report-page min-h-screen bg-gray-50 px-6 py-8 text-gray-900 dark:bg-dark-950 dark:text-gray-100" :class="{ capturing }">
    <div class="workspace-page qq-workspace space-y-5">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-200 pb-5 dark:border-dark-700">
        <div>
          <h1 class="text-2xl font-semibold">{{ report?.title || 'QQ 查询页面' }}</h1>
          <p v-if="report" class="mt-2 text-sm text-gray-500">查询于 {{ formatTime(report.created_at) }} · 链接有效至 {{ formatTime(report.expires_at) }}</p>
        </div>
        <a v-if="report" :href="report.source_path" class="text-sm text-primary-600">前往网站页面</a>
      </header>
      <p v-if="error" role="alert" class="rounded-lg border border-red-200 bg-white p-5 text-red-600">{{ error }}</p>
      <p v-else-if="!report" role="status">正在加载查询结果…</p>
      <template v-else>
        <p class="text-sm text-gray-500">这是本次查询的数据快照；最新结果请在群里重新查询。持有链接的人可查看本页。</p>
        <label v-if="report.kind !== 'balance'" class="block max-w-md text-sm">
          {{ report.kind === 'monitor' ? '筛选渠道或模型' : '筛选模型' }}
          <input v-model="filter" class="input mt-2 w-full" type="search" placeholder="输入关键词筛选" />
        </label>
        <section v-if="report.kind === 'monitor' && report.data.mode === 'v1'">
          <MonitorCardGrid :items="monitorItems" window="7d" :countdown-seconds="null" :loading="false" :detail-cache="{}" :quality-plans="report.data.quality_plans || []" @card-click="selectedMonitor = $event" @quality-select="selectedQuality = $event" @point-select="selectPoint" />
          <p v-if="selectedMonitor" class="mt-4 text-sm">{{ selectedMonitor.name }} · {{ selectedMonitor.primary_model }} · 点击上方历史方块查看时间与延迟，点击下方检测方块查看评分和作品。</p>
        </section>
        <section v-else-if="report.kind === 'monitor'" class="overflow-hidden rounded-lg border bg-white dark:bg-dark-900">
          <table class="w-full text-left text-sm">
            <thead><tr class="border-b"><th class="p-4">分组</th><th class="p-4">错误率</th><th class="p-4">首字延迟 P50</th></tr></thead>
            <tbody><tr v-for="group in passiveGroups" :key="group.Name" class="border-b"><td class="p-4">{{ group.Name }}</td><td class="p-4">{{ (group.Metrics.ErrorRate * 100).toFixed(1) }}%</td><td class="p-4">{{ group.Metrics.TTFTP50Ms ?? '—' }} ms</td></tr></tbody>
          </table>
          <p v-if="!passiveGroups.length" class="p-5 text-gray-500">当前没有可展示的监控样本。</p>
        </section>
        <section v-else-if="report.kind === 'balance'" class="space-y-6">
          <div class="mx-auto max-w-md rounded-xl border border-gray-200 bg-white p-8 dark:border-dark-700 dark:bg-dark-900">
            <h2 class="mb-6 text-sm font-semibold text-gray-500">钱包余额</h2>
            <div class="flex justify-center"><WalletBalanceValue :amount="money(report.data.balance)" /></div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <h2 class="border-b px-8 py-5 text-sm font-semibold text-gray-500 dark:border-dark-700">详细信息</h2>
            <dl class="divide-y divide-gray-100 dark:divide-dark-800">
              <div class="flex justify-between px-8 py-5"><dt>查询范围</dt><dd>本人绑定账号的钱包余额</dd></div>
              <div class="flex justify-between px-8 py-5"><dt>剩余余额</dt><dd class="font-semibold text-teal-500">{{ money(report.data.balance) }}</dd></div>
            </dl>
          </div>
        </section>
        <section v-else-if="report.kind === 'usage'">
          <ModelUsageTable :items="usageItems" />
          <p v-if="!usageItems.length" class="rounded-lg border bg-white p-6 text-gray-500 dark:bg-dark-900">暂无匹配的模型用量。</p>
        </section>
      </template>
      <ModelQualityDetails v-if="report?.kind === 'monitor'" :selection="selectedQuality" snapshot-only :load-results="loadQualityResults" @close="selectedQuality = null" />
      <BaseDialog :show="!!selectedPoint" :title="selectedPoint?.item.name || '检测详情'" width="normal" @close="selectedPoint = null"><dl v-if="selectedPoint" class="space-y-3 text-sm"><div>模型：{{ selectedPoint.item.primary_model }}</div><div>时间：{{ formatTime(selectedPoint.point.checked_at) }}</div><div>状态：{{ statusLabel(selectedPoint.point.status) }}</div><div>响应延迟：{{ selectedPoint.point.latency_ms ?? '—' }} ms</div><div>网络延迟：{{ selectedPoint.point.ping_latency_ms ?? '—' }} ms</div></dl></BaseDialog>
      <div v-if="ready" data-report-ready="true" aria-hidden="true" />
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { loadLocaleMessages } from '@/i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelQualityDetails from '@/features/model-quality/ModelQualityDetails.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import MonitorCardGrid from '@/components/user/monitor/MonitorCardGrid.vue'
import WalletBalanceValue from '@/components/common/WalletBalanceValue.vue'
import ModelUsageTable, { type ModelUsageRow } from '@/components/common/ModelUsageTable.vue'
import type { UserMonitorView, MonitorTimelinePoint } from '@/api/channelMonitor'
import type { QualityResults, QualityBenchmark, QualityPlan } from '@/api/modelQuality'
import type { QualitySelection } from '@/features/model-quality/qualityPresentation'

interface Report {
  kind: string; title: string; source_path: string; created_at: string; expires_at: string
  data: { mode?: string; items?: UserMonitorView[]; quality_plans?: QualityPlan[]; balance?: number; models?: ModelUsageRow[]; filter?: string; report?: { Groups?: { Name: string; Metrics: { ErrorRate: number; TTFTP50Ms: number | null } }[] } }
}
const report = ref<Report | null>(null), error = ref(''), filter = ref(''), ready = ref(false)
const selectedPoint = ref<{ item: UserMonitorView; point: MonitorTimelinePoint } | null>(null)
const selectedMonitor = ref<UserMonitorView | null>(null), selectedQuality = ref<QualitySelection | null>(null)
const capturing = new URLSearchParams(location.search).get('capture') === '1'
const matches = (value: string) => value.toLocaleLowerCase().includes(filter.value.trim().toLocaleLowerCase())
const monitorItems = computed(() => (report.value?.data.items || []).filter(item => matches([item.name, item.primary_model, item.group_name].join(' '))))
const usageItems = computed(() => (report.value?.data.models || []).filter(item => matches(item.model)))
const passiveGroups = computed(() => (report.value?.data.report?.Groups || []).filter(item => matches(item.Name)))
const money = (amount: number | undefined) => '$' + Number(amount || 0).toFixed(2)
const formatTime = (value: string) => new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })
const { statusLabel } = useChannelMonitorFormat()
function selectPoint(item: UserMonitorView, point: MonitorTimelinePoint) { selectedPoint.value = { item, point } }
async function loadQualityResults(selection: QualitySelection, filters: { model?: string; benchmark?: QualityBenchmark | ''; page?: number }): Promise<QualityResults> {
  const token = location.hash.slice(1)
  const response = await fetch('/api/v1/qq-bot/report/detail', { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'omit', cache: 'no-store', body: JSON.stringify({ token, plan_id: selection.planId, batch_id: selection.batchId, ...filters }), signal: controller?.signal })
  if (!response.ok) throw new Error('检测结果暂不可用或链接已过期，请在群里重新查询。')
  return response.json() as Promise<QualityResults>
}
let controller: AbortController | undefined
const { locale } = useI18n({ useScope: 'global' })
const previousLocale = locale.value
const previousLanguage = document.documentElement.lang
let expiryTimer: ReturnType<typeof setTimeout> | undefined
onUnmounted(() => {
  window.removeEventListener('hashchange', loadReport)
  controller?.abort(); clearTimeout(expiryTimer)
  locale.value = previousLocale
  document.documentElement.lang = previousLanguage
})
async function loadReport() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  clearTimeout(expiryTimer)
  report.value = null; error.value = ''; ready.value = false; filter.value = ''
  selectedMonitor.value = null; selectedQuality.value = null; selectedPoint.value = null
  try {
    // QQ commands and report copy are Chinese. Use the same language for the
    // shared components without overwriting the visitor's saved preference.
    await loadLocaleMessages('zh')
    if (request.signal.aborted) return
    locale.value = 'zh'
    document.documentElement.lang = 'zh'
    const token = location.hash.slice(1)
    if (!/^[a-f0-9]{64}$/.test(token)) throw new Error('页面链接不完整，请在群里重新查询。')
    const response = await fetch('/api/v1/qq-bot/report', { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'omit', cache: 'no-store', body: JSON.stringify({ token }), signal: request.signal })
    if (!response.ok) throw new Error('页面链接已失效，请在群里重新发送查询指令。')
    const loaded = await response.json() as Report
    if (request.signal.aborted) return
    report.value = loaded
    filter.value = report.value!.data.filter || ''
    expiryTimer = setTimeout(() => { report.value = null; selectedQuality.value = null; selectedPoint.value = null; ready.value = false; error.value = '页面链接已到期，请在群里重新查询。' }, Math.max(0, new Date(report.value!.expires_at).getTime() - Date.now()))
    await nextTick()
    await document.fonts.ready
    await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    if (!request.signal.aborted) ready.value = true
  } catch (cause) {
    if (!request.signal.aborted) error.value = cause instanceof Error ? cause.message : '页面加载失败，请稍后重试。'
  }
}
onMounted(() => {
  const referrer = document.createElement('meta'); referrer.name = 'referrer'; referrer.content = 'no-referrer'; document.head.appendChild(referrer)
  window.addEventListener('hashchange', loadReport)
  void loadReport()
})
</script>

<style scoped>
.capturing :deep(*) { animation: none !important; transition: none !important; }
</style>
