<template>
  <BaseDialog :show="true" :title="t(plan ? 'modelQuality.edit' : 'modelQuality.create')" width="wide" @close="emit('close')">
    <form id="quality-form" class="space-y-4" @submit.prevent="save">
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.name') }}</span>
          <input v-model="form.name" class="input" required maxlength="160" />
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.monitor') }}</span>
          <select v-model.number="monitorID" class="input" required :disabled="!!plan?.config?.monitor_id" @change="changeMonitor">
            <option :value="0" disabled>{{ t('modelQuality.selectMonitor') }}</option>
            <option v-for="monitor in options.monitors" :key="monitor.id" :value="monitor.id">{{ monitor.name }}</option>
            <option v-if="plan?.config?.monitor_id && !options.monitors.some(m => m.id === plan?.config?.monitor_id)" :value="plan.config.monitor_id">{{ plan.target.monitor_name || `#${plan.config.monitor_id}` }}</option>
          </select>
          <span v-if="!loading && !options.monitors.length" class="block text-xs text-amber-700 dark:text-amber-400">{{ t('modelQuality.noMonitors') }}</span>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.group') }}</span>
          <select v-model.number="form.group_id" class="input" required :disabled="!!plan || !!selectedMonitor?.group_id">
            <option :value="0" disabled>—</option>
            <option v-for="group in options.groups" :key="group.id" :value="group.id">{{ group.name }}{{ group.channel_name ? ` · ${group.channel_name}` : '' }}</option>
          </select>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.interval') }}</span>
          <input v-model.number="form.interval_minutes" type="number" class="input" min="3" max="10080" step="1" required />
        </label>
      </div>
      <div class="space-y-3">
        <div v-for="(row, index) in models" :key="row.key" class="space-y-3 border-t border-gray-200 pt-3 dark:border-dark-700" data-model-config>
          <div class="grid items-end gap-3 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto]">
            <label class="space-y-1 text-sm"><span>{{ t('modelQuality.model') }}</span><input v-model.trim="row.model" class="input" required maxlength="200" /></label>
            <label class="space-y-1 text-sm"><span>{{ t('modelQuality.effort') }}</span><select v-model="row.effort" class="input"><option v-for="effort in efforts" :key="effort" :value="effort">{{ effort }}</option></select></label>
            <label class="space-y-1 text-sm"><span>{{ t('modelQuality.tier') }}</span><select v-model="row.tier" class="input"><option value="">{{ t('modelQuality.defaultTier') }}</option><option value="priority">priority</option><option value="ultrafast">ultrafast</option></select></label>
            <button type="button" class="btn btn-ghost btn-sm" :disabled="models.length === 1" :aria-label="t('modelQuality.removeModel')" @click="models.splice(index, 1)">{{ t('modelQuality.remove') }}</button>
          </div>
          <fieldset class="flex flex-wrap gap-x-4 gap-y-2 text-sm">
            <legend class="mb-2 text-xs text-gray-500">{{ t('modelQuality.benchmark') }}</legend>
            <label v-for="mode in modes" :key="mode" class="flex items-center gap-2">
              <input v-model="row.modes" type="checkbox" :value="mode" :disabled="mode !== 'candy' && row.model !== 'gpt-6-astra' && !row.modes.includes(mode)" />{{ t(`modelQuality.benchmarks.${mode}`) }}
            </label>
          </fieldset>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="models.length >= 100" @click="addModel">{{ t('modelQuality.addModel') }}</button>
        <p class="text-xs text-gray-500">{{ t('modelQuality.modeSupport') }}</p>
        <p v-if="validationError" class="text-xs text-amber-700 dark:text-amber-400">{{ validationError }}</p>
      </div>
      <label class="flex items-center gap-2 text-sm"><input v-model="form.enabled" type="checkbox" />{{ t('modelQuality.enabled') }}</label>
      <label class="flex items-center gap-2 text-sm"><input v-model="form.public_visible" type="checkbox" />{{ t('modelQuality.publicVisible') }}</label>
      <p class="text-xs text-gray-500">{{ t('modelQuality.boundCredentialNotice') }}</p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('modelQuality.close') }}</button>
      <button type="submit" form="quality-form" class="btn btn-primary" :disabled="saving || loading || !form.group_id || !monitorID || !!validationError">{{ t('modelQuality.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import * as api from '@/api/modelQuality'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ plan: api.QualityPlan | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const saving = ref(false)
const loading = ref(true)
const error = ref('')
const options = ref<api.QualityOptions>({ groups: [], keys: [], monitors: [] })
const monitorID = ref(props.plan?.config?.monitor_id || props.plan?.target.monitor_id || 0)
const form = reactive({
  name: props.plan?.name ?? '', group_id: props.plan?.group_id ?? 0,
  enabled: props.plan?.enabled ?? false, public_visible: props.plan?.public_visible ?? false,
  interval_minutes: props.plan?.interval_minutes ?? 3,
})
const modes = ['candy', 'pelican', 'creative'] as const
const efforts = ['low', 'medium', 'high', 'xhigh', 'max', 'ultra']
type ModelRow = { key: number; model: string; effort: string; tier: string; modes: api.QualityConfig['benchmark'][] }
let nextKey = 0
const initial = props.plan?.configs?.length ? props.plan.configs : props.plan?.config ? [props.plan.config] : []
const grouped: ModelRow[] = []
for (const c of initial) {
  let row = grouped.find(r => r.model === c.model && r.effort === c.reasoning_effort && r.tier === c.service_tier)
  if (!row) { row = { key: nextKey++, model: c.model, effort: c.reasoning_effort, tier: c.service_tier, modes: [] }; grouped.push(row) }
  row.modes.push(c.benchmark)
}
const models = ref<ModelRow[]>(grouped.length ? grouped : [{ key: nextKey++, model: 'gpt-6-astra', effort: 'low', tier: '', modes: ['candy'] }])
const configs = computed<api.QualityConfig[]>(() => models.value.flatMap(row => row.modes.map(benchmark => ({
  monitor_id: monitorID.value, model: row.model.trim(), benchmark, reasoning_effort: row.effort, service_tier: row.tier,
}))))
const validationError = computed(() => {
  if (models.value.some(row => !row.model.trim() || !row.modes.length)) return t('modelQuality.selectModelModes')
  if (configs.value.length > 100) return t('modelQuality.tooManyConfigs')
  if (configs.value.some(c => c.benchmark !== 'candy' && (c.model !== 'gpt-6-astra' || !['low', 'medium', 'high'].includes(c.reasoning_effort)))) return t('modelQuality.modeSupport')
  const keys = configs.value.map(c => JSON.stringify([c.model, c.benchmark, c.reasoning_effort, c.service_tier]))
  return new Set(keys).size !== keys.length ? t('modelQuality.duplicateConfig') : ''
})
const selectedMonitor = computed(() => options.value.monitors.find(m => m.id === monitorID.value))
function changeMonitor() {
  if (!props.plan) {
    form.group_id = selectedMonitor.value?.group_id || 0
    if (models.value.length === 1 && models.value[0].modes.every(m => m === 'candy') && selectedMonitor.value?.model) models.value[0].model = selectedMonitor.value.model
  }
}
function addModel() { models.value.push({ key: nextKey++, model: '', effort: 'low', tier: '', modes: ['candy'] }) }
async function save() {
  if (saving.value || validationError.value || !monitorID.value || !form.group_id) return
  saving.value = true; error.value = ''
  try { await api.save(props.plan?.id ?? null, { ...form, configs: configs.value }); emit('saved') }
  catch (e) { error.value = extractApiErrorMessage(e, t('modelQuality.operationFailed')) }
  finally { saving.value = false }
}
onMounted(async () => {
  try {
    options.value = await api.options()
    if (!monitorID.value && props.plan?.monitor_ids?.length === 1) monitorID.value = props.plan.monitor_ids[0]
    if (monitorID.value) changeMonitor()
  }
  catch (e) { error.value = extractApiErrorMessage(e, t('modelQuality.loadFailed')) }
  finally { loading.value = false }
})
</script>
