<template>
  <BaseDialog :show="show" :title="t('monitorDisplay.title')" width="wide" :close-on-escape="!saving" :show-close-button="!saving" @close="emit('close')">
    <p class="mb-4 text-sm text-gray-500 dark:text-gray-400">{{ t('monitorDisplay.description') }}</p>
    <p v-if="loading" role="status">{{ t('common.loading') }}</p>
    <div v-else-if="loaded" class="space-y-4">
      <section v-for="(section, index) in sections" :key="section.provider" class="rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="flex items-center justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
          <h4 class="font-medium">{{ providerLabel(section.provider) }}</h4>
          <div class="flex gap-1">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || index === 0" :aria-label="t('monitorDisplay.moveUp', { name: providerLabel(section.provider) })" @click="move(sections, index, -1)"><Icon name="arrowUp" size="sm" /></button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || index === sections.length - 1" :aria-label="t('monitorDisplay.moveDown', { name: providerLabel(section.provider) })" @click="move(sections, index, 1)"><Icon name="arrowDown" size="sm" /></button>
          </div>
        </div>
        <div v-for="(monitor, rowIndex) in section.monitors" :key="monitor.id" class="flex flex-wrap items-center gap-3 border-b border-gray-100 px-4 py-3 last:border-0 dark:border-dark-700">
          <div class="min-w-36 flex-1 text-sm">
            {{ monitor.name }} <span v-if="!monitor.enabled" class="text-gray-400">({{ t('monitorDisplay.disabled') }})</span>
          </div>
          <label class="flex min-w-0 flex-1 flex-col gap-1 text-xs text-gray-500">
            {{ t('monitorDisplay.billingGroup') }}
            <select v-model="links[monitor.id]" class="input w-full" :disabled="saving" :aria-label="`${monitor.name} · ${t('monitorDisplay.billingGroup')}`">
              <option :value="0">{{ t('monitorDisplay.unlinked') }}</option>
              <option v-if="links[monitor.id] && !groups.some(group => group.id === links[monitor.id])" :value="links[monitor.id]">{{ t('monitorDisplay.missingGroup', { id: links[monitor.id] }) }}</option>
              <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.platform }} · {{ group.rate_multiplier }}×{{ group.status !== 'active' ? ` (${t('monitorDisplay.disabled')})` : '' }}</option>
            </select>
          </label>
          <div class="flex gap-1">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || rowIndex === 0" :aria-label="t('monitorDisplay.moveUp', { name: monitor.name })" @click="move(section.monitors, rowIndex, -1)"><Icon name="arrowUp" size="sm" /></button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || rowIndex === section.monitors.length - 1" :aria-label="t('monitorDisplay.moveDown', { name: monitor.name })" @click="move(section.monitors, rowIndex, 1)"><Icon name="arrowDown" size="sm" /></button>
          </div>
        </div>
        <p v-if="!section.monitors.length" class="px-4 py-3 text-sm text-gray-400">{{ t('monitorDisplay.noMonitors') }}</p>
      </section>
    </div>
    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button v-if="!loaded && !loading" type="button" class="btn btn-secondary" @click="load">{{ t('common.retry') }}</button>
      <button type="button" class="btn btn-primary" :disabled="loading || saving || !loaded" @click="save">{{ t('common.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { getDisplayConfig, updateDisplayConfig, type ChannelMonitor, type MonitorDisplayConfig } from '@/api/admin/channelMonitor'
import type { AdminGroup } from '@/types'
import { PROVIDERS } from '@/constants/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'saved'): void }>()
const { t } = useI18n()
const { providerLabel } = useChannelMonitorFormat()
const app = useAppStore()
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const sections = ref<Array<{ provider: string; monitors: ChannelMonitor[] }>>([])
const links = ref<Record<number, number>>({})
const groups = ref<AdminGroup[]>([])
let sequence = 0

function move<T>(items: T[], index: number, offset: number) {
  const target = index + offset
  if (target < 0 || target >= items.length) return
  const [item] = items.splice(index, 1)
  items.splice(target, 0, item!)
}

async function allMonitors() {
  const items: ChannelMonitor[] = []
  for (let page = 1; ; page++) {
    const result = await adminAPI.channelMonitor.list({ page, page_size: 100 })
    items.push(...result.items)
    if (items.length >= result.total || result.items.length === 0) return items
  }
}

async function load() {
  const request = ++sequence
  loading.value = true
  loaded.value = false
  try {
    const [config, monitors, billingGroups] = await Promise.all([
      getDisplayConfig(), allMonitors(), adminAPI.groups.getAllIncludingInactive(),
    ])
    if (request !== sequence) return
    groups.value = billingGroups
    links.value = Object.fromEntries(monitors.map(monitor => [monitor.id, config.group_mappings.find(link => link.monitor_id === monitor.id)?.group_id || 0]))
    const order = new Map(config.monitor_order.map((id, index) => [id, index]))
    const providers = [...new Set([...config.provider_order, ...PROVIDERS, ...monitors.map(monitor => monitor.provider)])]
    sections.value = providers.map(provider => ({
      provider,
      monitors: monitors.filter(monitor => monitor.provider === provider).sort((a, b) =>
        (order.get(a.id) ?? Infinity) - (order.get(b.id) ?? Infinity) || a.id - b.id),
    }))
    loaded.value = true
  } catch (error) {
    if (request === sequence) app.showError(extractApiErrorMessage(error, t('monitorDisplay.loadFailed')))
  } finally {
    if (request === sequence) loading.value = false
  }
}

async function save() {
  saving.value = true
  const monitorIDs = sections.value.flatMap(section => section.monitors.map(monitor => monitor.id))
  const config: MonitorDisplayConfig = {
    provider_order: sections.value.map(section => section.provider),
    monitor_order: monitorIDs,
    group_mappings: monitorIDs.filter(id => links.value[id] > 0).map(id => ({ monitor_id: id, group_id: links.value[id]! })),
  }
  try {
    await updateDisplayConfig(config)
    app.showSuccess(t('monitorDisplay.saved'))
    emit('saved')
    emit('close')
  } catch (error) {
    app.showError(extractApiErrorMessage(error, t('monitorDisplay.saveFailed')))
  } finally {
    saving.value = false
  }
}

watch(() => props.show, show => {
  if (show) void load()
  else sequence++
}, { immediate: true })
</script>
